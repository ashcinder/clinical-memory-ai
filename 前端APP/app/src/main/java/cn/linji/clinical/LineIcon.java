package cn.linji.clinical;

import android.content.Context;
import android.graphics.Canvas;
import android.graphics.Paint;
import android.graphics.Path;
import android.view.View;

/** Minimal line icons keep navigation legible without an icon-font or external library. */
final class LineIcon extends View {
    private final String name;
    private final Paint pen = new Paint(Paint.ANTI_ALIAS_FLAG);
    LineIcon(Context context, String name, int color) {
        super(context);
        this.name=name;
        pen.setColor(color);
        pen.setStyle(Paint.Style.STROKE);
        pen.setStrokeWidth(1.8f);
        pen.setStrokeCap(Paint.Cap.ROUND);
        pen.setStrokeJoin(Paint.Join.ROUND);
        setImportantForAccessibility(View.IMPORTANT_FOR_ACCESSIBILITY_NO);
    }
    @Override protected void onDraw(Canvas canvas) {
        super.onDraw(canvas);
        float scale=Math.min(getWidth(),getHeight())/24f;
        canvas.save();canvas.translate((getWidth()-24*scale)/2,(getHeight()-24*scale)/2);canvas.scale(scale,scale);
        Path path=new Path();
        switch(name) {
            case "检索":
                canvas.drawCircle(10.4f,10.4f,5.8f,pen);canvas.drawLine(14.7f,14.7f,19.5f,19.5f,pen);break;
            case "笔记":
                canvas.drawRoundRect(5,3.5f,19,20.5f,2.5f,2.5f,pen);canvas.drawLine(8.5f,8.5f,15.5f,8.5f,pen);canvas.drawLine(8.5f,12,15.5f,12,pen);canvas.drawLine(8.5f,15.5f,13.5f,15.5f,pen);break;
            case "材料":
                path.moveTo(3.5f,7);path.lineTo(3.5f,18);path.quadTo(3.5f,20,5.5f,20);path.lineTo(18.5f,20);path.quadTo(20.5f,20,20.5f,18);path.lineTo(20.5f,9);path.quadTo(20.5f,7,18.5f,7);path.lineTo(11,7);path.lineTo(9.2f,5);path.lineTo(5.5f,5);path.quadTo(3.5f,5,3.5f,7);canvas.drawPath(path,pen);break;
            case "对话":
                path.moveTo(5,5);path.quadTo(3.5f,5,3.5f,7);path.lineTo(3.5f,15);path.quadTo(3.5f,17,5.5f,17);path.lineTo(8,17);path.lineTo(8,20);path.lineTo(12,17);path.lineTo(18.5f,17);path.quadTo(20.5f,17,20.5f,15);path.lineTo(20.5f,7);path.quadTo(20.5f,5,18.5f,5);path.close();canvas.drawPath(path,pen);canvas.drawLine(8,10,16,10,pen);canvas.drawLine(8,13,13,13,pen);break;
            default:
                canvas.drawCircle(12,8,3.5f,pen);path.moveTo(5.5f,20);path.cubicTo(5.8f,15,8.5f,13.7f,12,13.7f);path.cubicTo(15.5f,13.7f,18.2f,15,18.5f,20);canvas.drawPath(path,pen);break;
        }
        canvas.restore();
    }
}
