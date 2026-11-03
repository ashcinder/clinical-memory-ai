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

// Field presentation changes do not rewrite existing note values.
type detailFieldSpec struct {
	Kind    string   `json:"kind"`
	Options []string `json:"options,omitempty"`
}

var detailFieldSpecs = map[string]detailFieldSpec{
	"患者姓名":      {Kind: "short"},
	"年龄":        {Kind: "short"},
	"病历号":       {Kind: "short"},
	"操作名称":      {Kind: "short"},
	"主题 / 概念":   {Kind: "short"},
	"复盘主题":      {Kind: "short"},
	"图谱主题":      {Kind: "short"},
	"解剖部位":      {Kind: "short"},
	"资料主题":      {Kind: "short"},
	"来源 / 作者":   {Kind: "short"},
	"资料来源":      {Kind: "short"},
	"参考来源":      {Kind: "short"},
	"证据 / 参考来源": {Kind: "short"},
	"性别":        {Kind: "single", Options: []string{"男", "女", "其他"}},
	"科室":        {Kind: "single", Options: []string{"内科", "外科", "儿科", "妇产科", "急诊科", "中医科", "康复科", "全科"}},
	"随访方式":      {Kind: "single", Options: []string{"门诊", "电话", "线上", "住院", "上门"}},
	"观察方向":      {Kind: "single", Options: []string{"前面观", "后面观", "侧面观", "上面观", "下面观"}},
	"疾病 / 诊断":   {Kind: "multi"},
	"既往史":       {Kind: "multi"},
	"过敏史":       {Kind: "multi"},
	"辅助检查":      {Kind: "multi", Options: []string{"血液检查", "尿液检查", "影像检查", "心电图", "病理检查"}},
	"治疗方案":      {Kind: "multi", Options: []string{"药物治疗", "手术治疗", "康复治疗", "中医治疗", "观察随访"}},
	"器械 / 材料":   {Kind: "multi"},
	"所属学科":      {Kind: "multi"},
	"关键结构":      {Kind: "multi"},
	"不良反应":      {Kind: "multi"},
	"用药情况":      {Kind: "multi"},
	"下次随访日期":    {Kind: "date"},
	"主诉":        {Kind: "long"},
	"现病史":       {Kind: "long"},
	"体格检查":      {Kind: "long"},
	"医嘱":        {Kind: "long"},
	"学习目标":      {Kind: "long"},
	"适用情境":      {Kind: "long"},
	"禁忌与风险":     {Kind: "long"},
	"操作前准备":     {Kind: "long"},
	"关键要点":      {Kind: "long"},
	"常见问题":      {Kind: "long"},
	"核心结论":      {Kind: "long"},
	"鉴别要点":      {Kind: "long"},
	"适用范围":      {Kind: "long"},
	"待确认问题":     {Kind: "long"},
	"事件背景":      {Kind: "long"},
	"预期目标":      {Kind: "long"},
	"实际结果":      {Kind: "long"},
	"关键决策":      {Kind: "long"},
	"经验与不足":     {Kind: "long"},
	"改进措施":      {Kind: "long"},
	"症状变化":      {Kind: "long"},
	"检查结果":      {Kind: "long"},
	"后续计划":      {Kind: "long"},
	"图像所见":      {Kind: "long"},
	"标注说明":      {Kind: "long"},
	"用途":        {Kind: "long"},
	"补充说明":      {Kind: "long"},
}
var detailPrimaryFields = map[string][]string{
	"record":    {"患者姓名", "年龄", "性别", "疾病 / 诊断", "主诉"},
	"tutorial":  {"操作名称", "学习目标", "器械 / 材料"},
	"knowledge": {"主题 / 概念", "所属学科", "核心结论"},
	"review":    {"复盘主题", "事件背景", "实际结果"},
	"followup":  {"患者姓名", "年龄", "性别", "疾病 / 诊断", "随访方式", "症状变化"},
	"atlas":     {"图谱主题", "解剖部位", "关键结构"},
	"other":     {"资料主题", "来源 / 作者", "用途"},
}
