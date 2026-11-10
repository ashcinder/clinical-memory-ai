package server

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type organizerCitation struct {
	Name         string `json:"name"`
	Quote        string `json:"quote"`
	AttachmentID string `json:"attachment_id,omitempty"`
}

type organizerBlock struct {
	Heading   string              `json:"heading,omitempty"`
	Text      string              `json:"text,omitempty"`
	Citations []organizerCitation `json:"citations,omitempty"`
	Media     []string            `json:"media,omitempty"`
}

func organizerMedia(mime string) bool {
	return strings.HasPrefix(mime, "image/") || strings.HasPrefix(mime, "audio/")
}

func validOrganizerQuote(sources []organizerSource, index int, quote string) bool {
	return index > 0 && index <= len(sources) && quote != "" && len([]rune(quote)) <= 1000 &&
		strings.Contains(sources[index-1].Text, quote)
}

var organizerFigureName = regexp.MustCompile(`^(图[0-9]+)(?:[^0-9]|$)`)
var organizerFigureRef = regexp.MustCompile(`图[0-9]+`)
var organizerDatePattern = regexp.MustCompile(`(?:^|[^0-9])((?:19|20)[0-9]{2})[-./年]?([0-9]{1,2})[-./月]?([0-9]{1,2})(?:日|[^0-9]|$)`)

func organizerDates(text string) []string {
	var dates []string
	// Leave a consumed separator available for the next match.
	for {
		match := organizerDatePattern.FindStringSubmatchIndex(text)
		if match == nil {
			break
		}
		year := text[match[2]:match[3]]
		month, _ := strconv.Atoi(text[match[4]:match[5]])
		day, _ := strconv.Atoi(text[match[6]:match[7]])
		date := fmt.Sprintf("%s-%02d-%02d", year, month, day)
		if _, err := time.Parse("2006-01-02", date); err == nil {
			dates = append(dates, date)
		}
		text = text[match[7]:]
	}
	return unique(dates)
}

// A caption index can supply the date for a numbered figure whose original
// filename only contains its figure number.
func organizerFigureDates(sources []organizerSource) map[string][]string {
	dates := map[string][]string{}
	for _, source := range sources {
		if organizerMedia(source.MIME) {
			continue
		}
		for _, line := range strings.FieldsFunc(source.Text, func(r rune) bool {
			return r == '\n' || r == '。' || r == '；'
		}) {
			figures := unique(organizerFigureRef.FindAllString(line, -1))
			if len(figures) != 1 {
				continue
			}
			if found := organizerDates(line); len(found) == 1 {
				figure := figures[0]
				dates[figure] = unique(append(dates[figure], found[0]))
			}
		}
	}
	return dates
}

// Recover the date or figure printed on the same source line when the model
// cites only a short phrase. Never extend an ambiguous or repeated quotation.
func organizerMediaContexts(sources []organizerSource, refs []organizerReference) []organizerReference {
	contexts := append([]organizerReference(nil), refs...)
	for _, ref := range refs {
		if !validOrganizerQuote(sources, ref.Source, ref.Quote) || strings.Contains(ref.Quote, "\n") {
			continue
		}
		source := sources[ref.Source-1].Text
		if strings.Count(source, ref.Quote) != 1 {
			continue
		}
		start := strings.Index(source, ref.Quote)
		left := strings.LastIndexByte(source[:start], '\n') + 1
		line, _, _ := strings.Cut(source[left:], "\n")
		if line != ref.Quote && len([]rune(line)) <= 600 {
			contexts = append(contexts, organizerReference{ref.Source, line})
		}
	}
	return contexts
}

func organizerMediaEvidence(filename string, refs []organizerReference, figureDates map[string][]string) (sameDate, sameFigure bool) {
	dates := organizerDates(filename)
	figure := organizerFigureName.FindStringSubmatch(filename)
	if len(dates) == 0 && len(figure) > 0 {
		dates = figureDates[figure[1]]
	}
	for _, ref := range refs {
		mentioned := organizerDates(ref.Quote)
		sameDate = sameDate || (len(dates) == 1 && len(mentioned) == 1 && mentioned[0] == dates[0])
		figures := unique(organizerFigureRef.FindAllString(ref.Quote, -1))
		sameFigure = sameFigure || (len(figure) > 0 && len(figures) == 1 && figures[0] == figure[1])
	}
	return sameDate, sameFigure
}

func organizerProcedureImage(filename string) bool {
	for _, token := range []string{"操作", "注射", "手术", "术中", "处置"} {
		if strings.Contains(filename, token) {
			return true
		}
	}
	return false
}

func organizerProcedureSection(section string) bool {
	for _, token := range []string{"处置", "操作", "干预", "治疗情况概述", "处理经过"} {
		if strings.Contains(section, token) {
			return true
		}
	}
	return false
}

func (a *App) loadOrganizerBody(note Note) []organizerBlock {
	var raw string
	err := a.DB.QueryRow(`SELECT JSON_REMOVE(snapshot,'$.inputs','$.approved','$.draft.body_blocks')
		FROM note_organizer_drafts WHERE note_id=? ORDER BY created_at DESC,id DESC LIMIT 1`, note.ID).Scan(&raw)
	if err == sql.ErrNoRows {
		return nil
	}
	check(err)
	var stored struct {
		Draft   organizerDraft    `json:"draft"`
		Sources []organizerSource `json:"sources"`
	}
	check(json.Unmarshal([]byte(raw), &stored))
	return organizerBody(note.Content, stored.Draft, stored.Sources, note.Attachments)
}

// Match evidence against the saved wording and section. A rewritten paragraph
// must not inherit a quotation that was reviewed for different wording.
func organizerBody(content string, draft organizerDraft, sources []organizerSource, files []Attachment) []organizerBlock {
	attached := make(map[string]Attachment, len(files))
	for _, file := range files {
		attached[file.ID] = file
	}
	type anchor struct {
		section, text string
	}
	type supported struct {
		item organizerItem
		refs []organizerReference
	}
	anchors := map[anchor]supported{}
	legacy := strings.Contains(draft.Content, "\n\n【材料来源】\n〔1〕")
	if legacy {
		_, appendix, _ := strings.Cut(draft.Content, "\n\n【材料来源】\n")
		content = strings.TrimSuffix(content, "\n\n【材料来源】\n"+appendix)
	}
	for i, item := range draft.Evidence {
		key := anchor{item.Section, item.Text}
		if legacy {
			key.text += fmt.Sprintf(" 〔%d〕", i+1)
		}
		anchors[key] = supported{item, append([]organizerReference{{item.Source, item.Quote}}, item.Support...)}
	}
	blocks := []organizerBlock{}
	placements := map[int]supported{}
	section := ""
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "【") && strings.HasSuffix(line, "】") {
			section = strings.TrimSuffix(strings.TrimPrefix(line, "【"), "】")
			blocks = append(blocks, organizerBlock{Heading: section})
			continue
		}
		block := organizerBlock{Text: line}
		if evidence, ok := anchors[anchor{section, line}]; ok {
			block.Text = evidence.item.Text
			seen := map[organizerReference]bool{}
			for _, ref := range evidence.refs {
				if !validOrganizerQuote(sources, ref.Source, ref.Quote) || seen[ref] {
					continue
				}
				seen[ref] = true
				source := sources[ref.Source-1]
				if source.AttachmentID != "" && attached[source.AttachmentID].ID == "" {
					continue
				}
				block.Citations = append(block.Citations, organizerCitation{source.Name, ref.Quote, source.AttachmentID})
			}
			evidence.refs = organizerMediaContexts(sources, evidence.refs)
			placements[len(blocks)] = evidence
		}
		blocks = append(blocks, block)
	}
	placed := map[string]bool{}
	figureDates := organizerFigureDates(sources)
	addMedia := func(index, requested int, explicit bool) {
		if index < 1 || index > len(sources) {
			return
		}
		file := attached[sources[index-1].AttachmentID]
		if !organizerMedia(file.MIME) || placed[file.ID] {
			return
		}
		best, bestScore, ambiguous := -1, 0, false
		procedure := strings.HasPrefix(file.MIME, "image/") && organizerProcedureImage(file.Filename)
		for i := range blocks {
			evidence, ok := placements[i]
			if !ok {
				continue
			}
			date, figure := organizerMediaEvidence(file.Filename, evidence.refs, figureDates)
			score := 0
			switch {
			case procedure && organizerProcedureSection(evidence.item.Section) && (figure || date):
				score = 5
			case figure:
				score = 4
			case explicit && i == requested && date && !procedure:
				score = 1
			}
			for _, cite := range blocks[i].Citations {
				if cite.AttachmentID == file.ID {
					score = 6
					break
				}
			}
			if score == 0 {
				continue
			}
			if score > bestScore || (score == bestScore && i == requested && explicit) {
				best, bestScore, ambiguous = i, score, false
			} else if score == bestScore && (!explicit || best != requested) {
				ambiguous = true
			}
		}
		if best >= 0 && !ambiguous {
			blocks[best].Media = append(blocks[best].Media, file.ID)
			placed[file.ID] = true
		}
	}
	for i := range blocks {
		for _, index := range placements[i].item.Media {
			addMedia(index, i, true)
		}
	}
	for i := range blocks {
		for _, ref := range placements[i].refs {
			addMedia(ref.Source, i, false)
		}
	}
	for i, source := range sources {
		if strings.HasPrefix(attached[source.AttachmentID].MIME, "image/") {
			addMedia(i+1, -1, false)
		}
	}

	return blocks
}
