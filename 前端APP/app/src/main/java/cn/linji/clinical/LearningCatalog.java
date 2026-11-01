package cn.linji.clinical;

import org.json.JSONArray;
import org.json.JSONObject;
import java.util.Arrays;
import static cn.linji.clinical.Json.*;

/** Purpose-based grouping. Original categories and note text are preserved. */
final class LearningCatalog {
    static JSONArray defaults(){
        JSONArray out=new JSONArray();
        String[][] values={
            {"record","诊疗记录","诊疗记录|诊疗笔记|临床记录|病例记录","诊疗记录","已记录过程"},
            {"tutorial","操作教程","操作教程|教程|操作流程|技能教程|临床技能","教程原文","操作步骤"},
            {"knowledge","知识要点","知识要点|知识|知识笔记|理论知识","知识原文","学习要点"},
            {"review","经验复盘","经验复盘|经验总结|复盘|学习心得","复盘原文","复盘条目"},
            {"followup","随访记录","随访记录|随访|复诊记录","随访原文","随访过程"},
            {"atlas","图谱资料","图谱资料|图谱|图片资料|解剖图谱","资料说明","核对要点"},
            {"other","其他资料","","原文内容","已记录条目"}
        };
        for(String[] v:values)out.put(obj("id",v[0],"title",v[1],"aliases",array(Arrays.asList(v[2].split("\\|"))),"content_label",v[3],"steps_label",v[4],"count",0));
        return out;
    }
    static JSONObject type(JSONArray types,JSONObject note){
        for(JSONObject t:objects(types)){if(str(t,"id").equals(str(note,"note_type")))return t;for(String alias:strings(array(t,"aliases")))if(alias.equals(str(note,"category")))return t;}
        return types.optJSONObject(types.length()-1);
    }
}
