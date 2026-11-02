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
        pen.setStrokeWidth(1.6f);
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
            case "图片":
                canvas.drawRoundRect(3,4,21,20,2,2,pen);canvas.drawCircle(8,9,1.5f,pen);path.moveTo(4,17);path.lineTo(9,12);path.lineTo(13,16);path.lineTo(16,13);path.lineTo(20,17);canvas.drawPath(path,pen);break;
            case "关闭":
                canvas.drawLine(7,7,17,17,pen);canvas.drawLine(17,7,7,17,pen);break;
            case "筛选":
                canvas.drawLine(4,7,20,7,pen);canvas.drawLine(7,12,17,12,pen);canvas.drawLine(10,17,14,17,pen);break;
            case "发送":
                canvas.drawLine(12,19,12,5,pen);path.moveTo(6,11);path.lineTo(12,5);path.lineTo(18,11);canvas.drawPath(path,pen);break;
            case "附件":
                path.moveTo(8,14);path.lineTo(15,7);path.cubicTo(18,4,22,8,19,11);path.lineTo(10,20);path.cubicTo(5,24,0,17,4,13);path.lineTo(13,4);canvas.drawPath(path,pen);break;
            case "录音":
                canvas.drawRoundRect(9,3,15,15,3,3,pen);path.moveTo(5,11);path.cubicTo(5,21,19,21,19,11);canvas.drawPath(path,pen);canvas.drawLine(12,19,12,23,pen);canvas.drawLine(8,23,16,23,pen);break;
            case "历史":
                canvas.drawRoundRect(4,4,20,20,3,3,pen);canvas.drawLine(9,4,9,20,pen);canvas.drawLine(13,9,17,9,pen);canvas.drawLine(13,13,17,13,pen);break;
            case "检索":
                canvas.drawCircle(10.4f,10.4f,5.8f,pen);canvas.drawLine(14.7f,14.7f,19.5f,19.5f,pen);break;
            case "笔记":
                canvas.drawRoundRect(5,3.5f,19,20.5f,2.5f,2.5f,pen);canvas.drawLine(8.5f,8.5f,15.5f,8.5f,pen);canvas.drawLine(8.5f,12,15.5f,12,pen);canvas.drawLine(8.5f,15.5f,13.5f,15.5f,pen);break;
            case "穴位":
                canvas.drawCircle(12,4,2,pen);path.moveTo(8,8);path.lineTo(16,8);path.lineTo(19,13);path.moveTo(8,8);path.lineTo(5,13);path.moveTo(9,9);path.lineTo(10,15);path.lineTo(8,22);path.moveTo(15,9);path.lineTo(14,15);path.lineTo(16,22);path.moveTo(10,15);path.lineTo(14,15);canvas.drawPath(path,pen);canvas.drawCircle(12,11,1,pen);break;
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
