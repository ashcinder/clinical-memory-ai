package cn.linji.clinical;

import android.graphics.Color;
import android.graphics.Typeface;
import android.text.Spannable;
import android.text.SpannableStringBuilder;
import android.text.style.BackgroundColorSpan;
import android.text.style.RelativeSizeSpan;
import android.text.style.StyleSpan;
import android.text.style.TypefaceSpan;
import android.text.style.URLSpan;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

/** Renders plain Markdown into native text spans, without interpreting HTML. */
final class ChatText {
    private static final Pattern INLINE=Pattern.compile("`([^`\\n]+)`|\\*\\*([^*\\n]+)\\*\\*|\\[([^\\]\\n]+)\\]\\((https?://[^\\s)]+)\\)");
    private static void span(SpannableStringBuilder out,Object value,int from,int to){if(to>from)out.setSpan(value,from,to,Spannable.SPAN_EXCLUSIVE_EXCLUSIVE);}
    private static void inline(SpannableStringBuilder out,String text){
        Matcher m=INLINE.matcher(text);int cursor=0;
        while(m.find()){
            out.append(text.substring(cursor,m.start()));int start=out.length();
            if(m.group(1)!=null){out.append(m.group(1));span(out,new TypefaceSpan("monospace"),start,out.length());span(out,new BackgroundColorSpan(Color.rgb(240,244,243)),start,out.length());}
            else if(m.group(2)!=null){out.append(m.group(2));span(out,new StyleSpan(Typeface.BOLD),start,out.length());}
            else{out.append(m.group(3));span(out,new URLSpan(m.group(4)),start,out.length());}
            cursor=m.end();
        }
        out.append(text.substring(cursor));
    }
    static CharSequence format(String source){
        SpannableStringBuilder out=new SpannableStringBuilder();boolean code=false;
        for(String raw:source.replace("\r\n","\n").split("\n",-1)){
            String trimmed=raw.trim();if(trimmed.startsWith("```")||trimmed.startsWith("~~~")){code=!code;if(out.length()>0&&out.charAt(out.length()-1)!='\n')out.append('\n');continue;}
            int start=out.length();
            if(code){out.append(raw);span(out,new TypefaceSpan("monospace"),start,out.length());span(out,new BackgroundColorSpan(Color.rgb(243,246,245)),start,out.length());}
            else if(trimmed.matches("^#{1,6}\\s+.*")){inline(out,trimmed.replaceFirst("^#{1,6}\\s+",""));span(out,new StyleSpan(Typeface.BOLD),start,out.length());span(out,new RelativeSizeSpan(1.12f),start,out.length());}
            else if(trimmed.matches("^[-*+]\\s+.*")){out.append("•  ");inline(out,trimmed.replaceFirst("^[-*+]\\s+",""));}
            else if(trimmed.startsWith(">")){out.append("│  ");inline(out,trimmed.replaceFirst("^>\\s?",""));}
            else if(!trimmed.matches("^[-_]{3,}$")){inline(out,raw);}
            out.append('\n');
        }
        if(out.length()>0)out.delete(out.length()-1,out.length());return out;
    }
}
