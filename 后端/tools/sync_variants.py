#!/usr/bin/env python3
"""Synchronize shared source only; variant configuration and user data stay local."""
import argparse, hashlib, shutil, sys
from datetime import datetime
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]

def shared_files(root):
    fixed = ['AGENTS.md', '多版本同步.md', '后端/tools/sync_variants.py',
             '后端/go.mod', '后端/go.sum', '后端/internal/server/schema.sql', '数据库数据/schema.sql']
    result = {Path(p) for p in fixed if (root / p).is_file()}
    for folder, patterns in [('后端/cmd', ['*.go']), ('后端/internal/server', ['*.go']),
                             ('前端APP/app/src/main/java', ['*.java']),
                             ('前端APP/app/src/main/res/values', ['*.xml'])]:
        for pattern in patterns:
            result.update(p.relative_to(root) for p in (root / folder).rglob(pattern) if p.is_file())
    # Explicit Web assets; never touch downloads, runtime settings, or generated output.
    for name in ['index.html', 'app.js', 'styles.css', 'demo-match.js', 'favicon.svg', 'demo-assets.json']:
        if (root / '前端Web' / name).is_file(): result.add(Path('前端Web') / name)
    for p in (root / '前端Web/demo-files').rglob('*'):
        if p.is_file(): result.add(p.relative_to(root))
    return sorted(result)

def digest(p): return hashlib.sha256(p.read_bytes()).hexdigest() if p.is_file() else None

def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument('--target', required=True, type=Path)
    parser.add_argument('--write', action='store_true', help='Back up and copy changed shared files')
    args = parser.parse_args(); target = args.target.expanduser().resolve()
    if target == ROOT or not (target / '后端/go.mod').is_file() or not (target / '前端APP/app/build.gradle').is_file():
        parser.error('Target must be another existing project workspace')
    changed = [p for p in shared_files(ROOT) if digest(ROOT/p) != digest(target/p)]
    if args.write and changed:
        backup = target/'后端/output/sync-backups'/datetime.now().strftime('%Y%m%d-%H%M%S-%f')
        for relative in changed:
            source, dest = ROOT/relative, target/relative
            if source.is_symlink() or dest.is_symlink(): raise RuntimeError('Refusing symlink: '+str(relative))
            if dest.exists():
                saved = backup/relative; saved.parent.mkdir(parents=True, exist_ok=True);shutil.copy2(dest,saved)
            dest.parent.mkdir(parents=True, exist_ok=True)
            temp = dest.with_name(dest.name+'.sync-next');shutil.copy2(source,temp);temp.replace(dest)
        print('Shared source updated:', len(changed), 'files; backup:', backup)
    else:
        print('Shared source differences:', len(changed))
        for p in changed: print(p)
    return 0 if args.write or not changed else 1
if __name__ == '__main__': sys.exit(main())
