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

func organizerMediaMatches(filename string, refs []organizerReference, figureDates map[string][]string) bool {
	dates := organizerDates(filename)
	figure := organizerFigureName.FindStringSubmatch(filename)
	if len(dates) == 0 && len(figure) > 0 {
		dates = figureDates[figure[1]]
	}
	for _, ref := range refs {
		mentioned := organizerDates(ref.Quote)
		if len(dates) == 1 && len(mentioned) == 1 && mentioned[0] == dates[0] {
			return true
		}
		figures := unique(organizerFigureRef.FindAllString(ref.Quote, -1))
		if len(figure) > 0 && len(figures) == 1 && figures[0] == figure[1] {
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
			placements[len(blocks)] = evidence
		}
		blocks = append(blocks, block)
	}
	placed := map[string]bool{}
	figureDates := organizerFigureDates(sources)
	addMedia := func(block *organizerBlock, index int, evidence supported, explicit bool) {
		if index < 1 || index > len(sources) {
			return
		}
		file := attached[sources[index-1].AttachmentID]
		if !organizerMedia(file.MIME) || placed[file.ID] {
			return
		}
		cited := false
		for _, cite := range block.Citations {
			cited = cited || cite.AttachmentID == file.ID
		}
		if !cited && (!explicit || !organizerMediaMatches(file.Filename, evidence.refs, figureDates)) {
			return
		}
		block.Media = append(block.Media, file.ID)
		placed[file.ID] = true
	}
	for i := range blocks {
		evidence := placements[i]
		for _, index := range evidence.item.Media {
			addMedia(&blocks[i], index, evidence, true)
		}
	}
	for i := range blocks {
		evidence := placements[i]
		for _, ref := range evidence.refs {
			addMedia(&blocks[i], ref.Source, evidence, false)
		}
	}
	return blocks
}
