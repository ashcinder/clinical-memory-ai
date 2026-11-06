#!/usr/bin/env python3
"""Evaluate an existing library against human-labelled image queries.
Creates and deletes only its own unattached query uploads. Never modifies notes.
"""
import argparse
import json
import math
import mimetypes
import os
from pathlib import Path
import statistics
import time
import urllib.error
import urllib.request
import uuid


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('manifest', type=Path, help='JSON {cases:[{image,expected_note_ids,category?,note_types?,tags?}]}')
    parser.add_argument('--base-url', default='http://localhost:8001')
    parser.add_argument('--output', required=True, type=Path)
    args = parser.parse_args()
    username = os.environ.get('LINJI_EVAL_USERNAME')
    password = os.environ.get('LINJI_EVAL_PASSWORD')
    if not username or not password:
        parser.error('Set LINJI_EVAL_USERNAME and LINJI_EVAL_PASSWORD; credentials are never written to the report')
    manifest = json.loads(args.manifest.read_text())
    cases = manifest['cases']
    if not cases:
        parser.error('Manifest must contain labelled cases')
    for case in cases:
        if not isinstance(case.get('expected_note_ids'), list):
            parser.error('Every case requires expected_note_ids; [] means no matching record')
        if Path(case['image']).suffix.lower() not in ('.png', '.jpg', '.jpeg', '.webp'):
            parser.error('Only JPG/PNG/WebP queries are supported')
        if not (args.manifest.parent / case['image']).is_file():
            parser.error('Query image does not exist: ' + case['image'])
    token = ''
    base = args.base_url.rstrip('/')

    def call(path, data=None, method=None, content_type='application/json'):
        payload = json.dumps(data).encode() if data is not None and content_type == 'application/json' else data
        headers = {'Content-Type': content_type}
        if token:
            headers['Authorization'] = 'Bearer ' + token
        req = urllib.request.Request(base + '/api' + path, data=payload, headers=headers, method=method)
        with urllib.request.urlopen(req, timeout=185) as response:
            return json.load(response)

    token = call('/auth/login', {'username': username, 'password': password})['token']
    results = []
    for i, case in enumerate(cases):
        uploaded_id = None
        start = time.monotonic()
        result = {'case': case.get('name', str(i + 1)), 'expected_note_ids': case['expected_note_ids']}
        try:
            path = args.manifest.parent / case['image']
            boundary = 'LinjiEval' + uuid.uuid4().hex
            mime = mimetypes.guess_type(path.name)[0] or 'application/octet-stream'
            # Fixed file name avoids multipart header injection through a manifest.
            name = 'query' + path.suffix.lower()
            body = (f'--{boundary}\r\nContent-Disposition: form-data; name="file"; filename="{name}"\r\nContent-Type: {mime}\r\n\r\n').encode() + path.read_bytes() + f'\r\n--{boundary}--\r\n'.encode()
            uploaded = call('/attachments', body, 'POST', 'multipart/form-data; boundary=' + boundary)
            uploaded_id = uploaded['id']
            query_start = time.monotonic()
            search = call('/search', {**{key: case[key] for key in ('query', 'category', 'note_types', 'tags') if key in case}, 'attachment_ids': [uploaded_id], 'limit': 20})
            ids = [item['id'] for item in search['items']]
            expected = set(case['expected_note_ids'])
            result.update({'returned_ids': ids, 'top1_hit': bool(ids and ids[0] in expected) if expected else None,
                           'hit_at_5': bool(expected.intersection(ids[:5])) if expected else None,
                           'recall_at_5': len(expected.intersection(ids[:5])) / len(expected) if expected else None,
                           'correct_rejection': not ids if not expected else None,
                           'search_seconds': time.monotonic() - query_start, 'total_seconds': time.monotonic() - start,
                           'warnings': search.get('warnings', []), 'engine': search.get('engine'),
                           'unverified_matches': sum(m.get('method') == 'perceptual' for item in search['items'] for m in item.get('search_match', {}).get('image_matches', [])),
                           'extraction_ready': uploaded.get('status') == 'ready'})
        except (urllib.error.URLError, ValueError, KeyError) as error:
            result['error'] = type(error).__name__ + (': HTTP ' + str(error.code) if isinstance(error, urllib.error.HTTPError) else '')
        finally:
            if uploaded_id:
                try:
                    call('/attachments/' + uploaded_id, method='DELETE')
                except (urllib.error.URLError, ValueError):
                    result['cleanup_failed_attachment_id'] = uploaded_id
        results.append(result)
        print(f"Evaluated {i + 1}/{len(cases)}", flush=True)
    positive = [r for r in results if r['expected_note_ids']]
    negative = [r for r in results if not r['expected_note_ids']]
    valid_negative = [r for r in negative if 'returned_ids' in r]
    durations = sorted(r['search_seconds'] for r in results if 'search_seconds' in r)
    report = {'cases': results, 'summary': {
        'queries': len(results), 'positive_queries': len(positive), 'negative_queries': len(negative),
        'top1_hit_rate': sum(bool(r.get('top1_hit')) for r in positive) / len(positive) if positive else None,
        'hit_at_5': sum(bool(r.get('hit_at_5')) for r in positive) / len(positive) if positive else None,
        'mean_recall_at_5': sum(r.get('recall_at_5', 0) for r in positive) / len(positive) if positive else None,
        'no_answer_false_positive_rate': sum(bool(r['returned_ids']) for r in valid_negative) / len(valid_negative) if valid_negative else None,
        'correct_rejection_rate': sum(bool(r.get('correct_rejection')) for r in negative) / len(negative) if negative else None,
        'negative_errors': len(negative) - len(valid_negative),
        'errors': sum('error' in r for r in results), 'cleanup_failures': sum('cleanup_failed_attachment_id' in r for r in results),
        'p50_search_seconds': statistics.median(durations) if durations else None,
        'p95_search_seconds': durations[math.ceil(len(durations) * .95) - 1] if durations else None,
        'warning_queries': sum(bool(r.get('warnings')) for r in results),
        'unverified_matches': sum(r.get('unverified_matches', 0) for r in results),
    }}
    args.output.parent.mkdir(parents=True, exist_ok=True)
    descriptor = os.open(args.output, os.O_WRONLY | os.O_CREAT | os.O_TRUNC, 0o600)
    with os.fdopen(descriptor, 'w') as stream:
        json.dump(report, stream, ensure_ascii=False, indent=2)
    print(json.dumps(report['summary'], ensure_ascii=False))
    return 1 if report['summary']['errors'] or report['summary']['cleanup_failures'] else 0


if __name__ == '__main__':
    raise SystemExit(main())
