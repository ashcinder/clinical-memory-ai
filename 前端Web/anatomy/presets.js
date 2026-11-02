// Approximate display anchors only. Never clinically calibrated or suitable for needle placement.
// Each sex is projected onto its own mesh; saved coordinates are never transferred between meshes.
export const MODELS = {
 male: {id:'male-real-v1',label:'男性',file:'male.glb'},
 female: {id:'female-real-v1',label:'女性',file:'female.glb'}
};
export function presetAnchors(sex){
 const central=[
 ['百会','GV20',0,1.995,0,'督脉'],['印堂','EX-HN3',0,1.85,1,'经外奇穴'],
 ['膻中','CV17',0,1.43,1,'任脉'],['中脘','CV12',0,1.19,1,'任脉'],
 ['神阙','CV8',0,1.09,1,'任脉'],['关元','CV4',0,.97,1,'任脉'],
 ['大椎','GV14',0,1.56,-1,'督脉'],['命门','GV4',0,1.13,-1,'督脉']
 ];
 const paired=[
 ['太阳','EX-HN5',.085,1.84,1,'经外奇穴'],['迎香','LI20',.027,1.79,1,'手阳明大肠经'],
 ['风池','GB20',.07,1.69,-1,'足少阳胆经'],['肩井','GB21',.20,1.55,1,'足少阳胆经'],
 ['中府','LU1',.17,1.49,1,'手太阴肺经'],['曲池','LI11',.40,1.25,1,'手阳明大肠经'],
 ['手三里','LI10',.44,1.18,1,'手阳明大肠经'],['内关','PC6',.49,1.07,1,'手厥阴心包经'],
 ['神门','HT7',.51,1.015,1,'手少阴心经'],['合谷','LI4',.555,.94,1,'手阳明大肠经'],
 ['血海','SP10',.115,.65,1,'足太阴脾经'],['犊鼻','ST35',.17,.57,1,'足阳明胃经'],
 ['足三里','ST36',.18,.49,1,'足阳明胃经'],['三阴交','SP6',.105,.20,1,'足太阴脾经'],
 ['太溪','KI3',.10,.12,1,'足少阴肾经'],['太冲','LR3',.14,.07,1,'足厥阴肝经'],
 ['肺俞','BL13',.075,1.47,-1,'足太阳膀胱经'],['肾俞','BL23',.08,1.13,-1,'足太阳膀胱经'],
 ['委中','BL40',.15,.55,-1,'足太阳膀胱经'],['承山','BL57',.15,.33,-1,'足太阳膀胱经'],
 ['昆仑','BL60',.19,.12,-1,'足太阳膀胱经']
 ];
 const model=MODELS[sex].id;
 return [...central.map(([name,code,x,y,face,meridian])=>({name,code,x,y,face,meridian,side:'正中'})),
 ...paired.flatMap(([name,code,x,y,face,meridian])=>[-1,1].map(sign=>({name,code,x:sign*x,y,face,meridian,side:sign>0?'左':'右'})))].map((p,i)=>({...p,id:'preset-'+model+'-'+i,model,description:'预置学习标记，三维位置未经专业校核，请结合权威图谱核对。',source:'穴位名称与编号参考常用命名；本模型坐标为交互示意，非标准定位数据。'}));
}
