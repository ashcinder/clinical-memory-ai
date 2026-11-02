package server

import (
	"database/sql"
	"encoding/json"
	"math"
	"net/http"
	"strconv"
)

type AnatomyPoint struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Code        string     `json:"code"`
	Side        string     `json:"side"`
	Meridian    string     `json:"meridian"`
	Description string     `json:"description"`
	Source      string     `json:"source"`
	Position    [3]float64 `json:"position"`
}
type AnatomyAtlas struct {
	Revision string         `json:"revision"`
	Points   []AnatomyPoint `json:"points"`
}

func validateAtlas(points []AnatomyPoint) {
	if len(points) > 500 {
		fail(422, "最多保存 500 个标注")
	}
	ids := map[string]bool{}
	for _, p := range points {
		bounded("标注ID", p.ID, 1, 64)
		bounded("名称", p.Name, 1, 80)
		bounded("编号", p.Code, 0, 40)
		bounded("侧别", p.Side, 0, 20)
		bounded("经络", p.Meridian, 0, 80)
		bounded("说明", p.Description, 0, 4000)
		bounded("来源", p.Source, 0, 1000)
		if ids[p.ID] {
			fail(422, "标注ID重复")
		}
		ids[p.ID] = true
		for _, v := range p.Position {
			if math.IsNaN(v) || math.IsInf(v, 0) || math.Abs(v) > 3 {
				fail(422, "标注位置超出模型范围")
			}
		}
	}
}
func (a *App) getAnatomy(w http.ResponseWriter, r *http.Request, u User) {
	var rev uint64
	var raw string
	check(a.DB.QueryRow("SELECT revision,points FROM anatomy_atlas WHERE id=1").Scan(&rev, &raw))
	points := []AnatomyPoint{}
	check(json.Unmarshal([]byte(raw), &points))
	respond(w, 200, AnatomyAtlas{strconv.FormatUint(rev, 10), points})
}
func (a *App) putAnatomy(w http.ResponseWriter, r *http.Request, u User) {
	var in AnatomyAtlas
	decode(r, &in)
	validateAtlas(in.Points)
	if in.Points == nil {
		in.Points = []AnatomyPoint{}
	}
	result := withTx(a, func(tx *sql.Tx) AnatomyAtlas {
		var rev uint64
		check(tx.QueryRow("SELECT revision FROM anatomy_atlas WHERE id=1 FOR UPDATE").Scan(&rev))
		if in.Revision != strconv.FormatUint(rev, 10) {
			fail(409, "图谱已被其他设备修改，请重新载入后再编辑")
		}
		_, err := tx.Exec("UPDATE anatomy_atlas SET revision=revision+1,points=? WHERE id=1", jsonText(in.Points))
		check(err)
		return AnatomyAtlas{strconv.FormatUint(rev+1, 10), in.Points}
	})
	respond(w, 200, result)
}
