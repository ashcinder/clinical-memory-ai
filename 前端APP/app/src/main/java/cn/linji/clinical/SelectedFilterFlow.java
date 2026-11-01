package cn.linji.clinical;

import android.content.Context;
import android.view.View;
import android.view.ViewGroup;

/** Wraps selected conditions beside their count, using the actual available width. */
final class SelectedFilterFlow extends ViewGroup {
    private final int gap;
    SelectedFilterFlow(Context context,int gap){super(context);this.gap=gap;}
    @Override protected LayoutParams generateDefaultLayoutParams(){return new LayoutParams(LayoutParams.WRAP_CONTENT,LayoutParams.WRAP_CONTENT);}
    @Override protected void onMeasure(int widthSpec,int heightSpec){
        int width=MeasureSpec.getSize(widthSpec);
        if(MeasureSpec.getMode(widthSpec)==MeasureSpec.UNSPECIFIED)width=getResources().getDisplayMetrics().widthPixels;
        int available=Math.max(0,width-getPaddingLeft()-getPaddingRight()),x=0,y=0,rowHeight=0;
        for(int i=0;i<getChildCount();i++){
            View child=getChildAt(i);if(child.getVisibility()==GONE)continue;
            child.measure(MeasureSpec.makeMeasureSpec(available,MeasureSpec.AT_MOST),MeasureSpec.makeMeasureSpec(0,MeasureSpec.UNSPECIFIED));
            int childWidth=child.getMeasuredWidth(),childHeight=child.getMeasuredHeight();
            if(x>0&&x+childWidth>available){x=0;y+=rowHeight+gap;rowHeight=0;}
            x+=childWidth+gap;rowHeight=Math.max(rowHeight,childHeight);
        }
        setMeasuredDimension(resolveSize(width,widthSpec),resolveSize(y+rowHeight+getPaddingTop()+getPaddingBottom(),heightSpec));
    }
    @Override protected void onLayout(boolean changed,int left,int top,int right,int bottom){
        int available=Math.max(0,right-left-getPaddingLeft()-getPaddingRight()),x=0,y=0,rowHeight=0;
        for(int i=0;i<getChildCount();i++){
            View child=getChildAt(i);if(child.getVisibility()==GONE)continue;
            int width=child.getMeasuredWidth(),height=child.getMeasuredHeight();
            if(x>0&&x+width>available){x=0;y+=rowHeight+gap;rowHeight=0;}
            child.layout(getPaddingLeft()+x,getPaddingTop()+y,getPaddingLeft()+x+width,getPaddingTop()+y+height);
            x+=width+gap;rowHeight=Math.max(rowHeight,height);
        }
    }
}
