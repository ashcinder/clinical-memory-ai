package server

import (
	"sort"
	"strings"
)

// Shared templates are delivered to both Web and Java clients.
var detailTemplates = map[string][]string{
	"record":    {"患者姓名", "年龄", "性别", "病历号", "科室", "主诉", "现病史", "既往史", "过敏史", "体格检查", "辅助检查", "疾病 / 诊断", "治疗方案", "医嘱"},
	"tutorial":  {"操作名称", "学习目标", "适用情境", "禁忌与风险", "器械 / 材料", "操作前准备", "关键要点", "常见问题", "参考来源"},
	"knowledge": {"主题 / 概念", "所属学科", "核心结论", "鉴别要点", "适用范围", "证据 / 参考来源", "待确认问题"},
	"review":    {"复盘主题", "事件背景", "预期目标", "实际结果", "关键决策", "经验与不足", "改进措施"},
	"followup":  {"患者姓名", "年龄", "性别", "病历号", "疾病 / 诊断", "随访方式", "症状变化", "用药情况", "不良反应", "检查结果", "后续计划", "下次随访日期"},
	"atlas":     {"图谱主题", "解剖部位", "观察方向", "关键结构", "图像所见", "标注说明", "资料来源"},
	"other":     {"资料主题", "来源 / 作者", "用途", "补充说明"},
}

func detailsText(details map[string]string) string {
	keys := make([]string, 0, len(details))
	for k := range details {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	lines := []string{}
	for _, k := range keys {
		lines = append(lines, k+"："+details[k])
	}
	return strings.Join(lines, "\n")
}
func validateDetails(details map[string]string) {
	if len(details) > 100 {
		fail(422, "专属信息最多 100 个字段")
	}
	total := 0
	for k, v := range details {
		bounded("字段名称", k, 1, 60)
		if strings.TrimSpace(k) != k {
			fail(422, "字段名称首尾不能包含空格")
		}
		bounded("字段内容", v, 0, 4000)
		total += len([]rune(v))
	}
	if total > 50000 {
		fail(422, "专属信息内容过长")
	}
}
