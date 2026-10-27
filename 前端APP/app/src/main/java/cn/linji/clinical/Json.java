package cn.linji.clinical;

import org.json.JSONArray;
import org.json.JSONException;
import org.json.JSONObject;
import java.util.ArrayList;
import java.util.Collection;
import java.util.List;

/** Small typed accessors keep checked JSON exceptions out of Android UI callbacks. */
final class Json {
    static JSONObject obj(Object... pairs) {
        JSONObject result = new JSONObject();
        for (int i=0; i<pairs.length; i+=2) put(result, (String)pairs[i], pairs[i+1]);
        return result;
    }
    static JSONObject put(JSONObject target, String key, Object value) {
        try { target.put(key, value == null ? JSONObject.NULL : value); return target; }
        catch (JSONException e) { throw new IllegalArgumentException("JSON 字段无效", e); }
    }
    static String str(JSONObject o, String key) { return o == null || o.isNull(key) ? "" : o.optString(key, ""); }
    static JSONArray array(JSONObject o, String key) { JSONArray a=o.optJSONArray(key); return a==null?new JSONArray():a; }
    static List<JSONObject> objects(JSONArray array) {
        List<JSONObject> result=new ArrayList<>();
        for(int i=0;i<array.length();i++){JSONObject o=array.optJSONObject(i);if(o!=null)result.add(o);}
        return result;
    }
    static List<String> strings(JSONArray array) { List<String> r=new ArrayList<>();for(int i=0;i<array.length();i++)r.add(array.optString(i));return r; }
    static JSONArray array(Collection<?> values) { return new JSONArray(values); }
    static String id(JSONObject o) { return str(o,"id"); }
    static String cut(String s, int max) { return s.length()>max?s.substring(0,max):s; }
}
