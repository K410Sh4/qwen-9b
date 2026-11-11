/* Jarvis Face Mesh 2.0 — stylized low-poly face, offline Canvas2D renderer.
   Geometry derived from MediaPipe canonical face mesh (Apache 2.0).
   Facial expressions are procedural and are NOT phoneme-accurate lip sync. */
(function () {
  "use strict";
  const geometry = window.jarvisMeshGeometry;
  const canvas = document.getElementById("faceMesh");
  if (!canvas || !geometry || !Array.isArray(geometry.positions) ||
      !Array.isArray(geometry.triangles) || geometry.positions.length !== 1404 ||
      geometry.triangles.length !== 2694) {
    throw new Error("Face mesh canônica inválida");
  }
  const ctx = canvas.getContext("2d", { alpha: true });
  if (!ctx) throw new Error("Canvas de avatar indisponível");

  const W = 256, H = 304, V = 468;
  const positions = Float32Array.from(geometry.positions);
  const indices = Uint16Array.from(geometry.triangles);
  const rotated = Array.from({ length: V }, () => ({ x:0, y:0, z:0 }));
  const projected = Array.from({ length: V }, () => ({ x:0, y:0, z:0 }));
  const triangles = Array.from({ length: indices.length/3 }, (_, i) => ({
    a: indices[3*i], b: indices[3*i+1], c: indices[3*i+2], z:0, light:0
  }));
  const outline = [10,338,297,332,284,251,389,356,454,323,361,288,397,
    365,379,378,400,377,152,148,176,149,150,136,172,58,132,93,234,
    127,162,21,54,103,67,109];
  const eyes = [
    { outer:33, inner:133, upper:159, lower:145, brow:[70,63,105,66,107],
      upperIds:[159,158,160,161,157], lowerIds:[145,144,153,154,155] },
    { outer:263, inner:362, upper:386, lower:374, brow:[336,296,334,293,300],
      upperIds:[386,387,385,384,388], lowerIds:[374,373,380,381,382] }
  ];
  const upperLids = new Set(eyes.flatMap(e => e.upperIds));
  const lowerLids = new Set(eyes.flatMap(e => e.lowerIds));
  const upperLip = new Set([61,185,40,39,37,0,267,269,270,409,291,13,312,311,310,415]);
  const lowerLip = new Set([146,91,181,84,17,314,405,321,375,14,87,178,88,95,317,402]);
  const reduceMotion = !!(window.matchMedia &&
    window.matchMedia("(prefers-reduced-motion: reduce)").matches);
  const clamp = (v, lo, hi) => Math.max(lo, Math.min(hi, v));
  const lerp = (a,b,t) => a + (b-a)*t;
  const presets = {
    ready:     { brow:.06, eye:.07, smile:.13, gazeX:0,    gazeY:0,    yaw:0,    pitch:0,     jaw:0,   cheek:.08 },
    listening: { brow:.26, eye:.24, smile:.05, gazeX:0,    gazeY:-.08, yaw:-.025,pitch:-.045, jaw:0,   cheek:.05 },
    thinking:  { brow:.12, eye:-.05,smile:0,   gazeX:.19,  gazeY:-.12, yaw:.10,  pitch:.025,  jaw:0,   cheek:0 },
    speaking:  { brow:.14, eye:.11, smile:.09, gazeX:0,    gazeY:0,    yaw:0,    pitch:-.008,jaw:.26, cheek:.07 }
  };
  const pose = { ...presets.ready };
  let target = presets.ready, activity = "ready", lastTime = 0, frameTime = -1;
  let active = true, framePending = false;

  // Coordinates must remain based on the canonical mesh to avoid detached features.
  function expression(seconds, dt) {
    const blend = reduceMotion ? 1 : 1 - Math.exp(-dt*7);
    for (const key of Object.keys(pose)) pose[key] = lerp(pose[key], target[key], blend);
    const speech = activity === "speaking" && !reduceMotion
      ? (.24 + .76*Math.pow(Math.sin(seconds*9.1),2)) *
        (.75 + .25*Math.sin(seconds*3.3+1)) : (activity === "speaking" ? .55 : 0);
    const blinkPhase = seconds % 5.1;
    const blink = reduceMotion ? 0 : blinkPhase < .17
      ? Math.sin(Math.PI * blinkPhase/.17) : 0;
    return { speech:clamp(speech,0,1), blink };
  }

  function buildFace(seconds, e) {
    const yaw = pose.yaw + (reduceMotion ? 0 : Math.sin(seconds*.37)*.035);
    const pitch = pose.pitch + (reduceMotion ? 0 : Math.sin(seconds*.27)*.014);
    const cy=Math.cos(yaw), sy=Math.sin(yaw), cp=Math.cos(pitch), sp=Math.sin(pitch);
    for (let i=0;i<V;i++) {
      let x=positions[i*3], y=positions[i*3+1], z=positions[i*3+2];
      const ax=Math.abs(x);
      // Retain facial proportions; introduce an angular jaw without deforming eyes.
      if(y>4.1) x*=.945;
      if(y< -2.6 && ax>2.0) x*=1.035;
      if(y< -5.5) x*=.94;
      if(y< -1.7 && y> -3.3 && ax>2 && ax<4.8) z+=.12 + pose.cheek*.14;
      if(ax<1.05 && y>-.9 && y<2.8) z+=.13;
      // Continuous local deformation; lips and jaw follow their neighboring triangles.
      const jawWeight=clamp((-y-3.65)/4.7,0,1);
      y -= jawWeight * pose.jaw * e.speech * .68;
      if(upperLip.has(i)) y+=.10*e.speech*pose.jaw;
      if(lowerLip.has(i)) y-=.41*e.speech*pose.jaw;
      if(i===61 || i===291) y+=pose.smile*.18;
      if(upperLids.has(i)) y+=pose.eye*.35 - e.blink*.40;
      if(lowerLids.has(i)) y-=pose.eye*.25 - e.blink*.30;
      const rx=x*cy+z*sy, rz=z*cy-x*sy;
      const ry=y*cp-rz*sp, zz=rz*cp+y*sp;
      const perspective=27/(27-zz), scale=11.3*perspective;
      rotated[i].x=rx;rotated[i].y=ry;rotated[i].z=zz;
      projected[i].x=W/2+rx*scale;
      projected[i].y=143-ry*scale;
      projected[i].z=zz;
    }
  }

  function path(ids, stroke, width=1, fill=null) {
    const first=projected[ids[0]];
    ctx.beginPath();ctx.moveTo(first.x,first.y);
    for(let i=1;i<ids.length;i++) ctx.lineTo(projected[ids[i]].x,projected[ids[i]].y);
    if(fill){ctx.closePath();ctx.fillStyle=fill;ctx.fill();}
    if(stroke){ctx.lineWidth=width;ctx.strokeStyle=stroke;ctx.stroke();}
  }

  function glow(x,y,r,color,blur=10) {
    ctx.save();ctx.shadowBlur=blur;ctx.shadowColor=color;ctx.fillStyle=color;
    ctx.beginPath();ctx.arc(x,y,r,0,Math.PI*2);ctx.fill();ctx.restore();
  }

  function backdrop(seconds) {
    ctx.clearRect(0,0,W,H);
    const energy=activity==="thinking"?.19:activity==="speaking"?.16:.10;
    const halo=ctx.createRadialGradient(128,147,12,128,147,142);
    halo.addColorStop(0,"rgba(11,52,73,"+(energy+.18)+")");
    halo.addColorStop(.7,"rgba(6,32,55,"+energy+")");
    halo.addColorStop(1,"rgba(0,0,0,0)");
    ctx.fillStyle=halo;ctx.fillRect(0,0,W,H);
    ctx.beginPath();ctx.ellipse(128,151,105,134,0,-1.3,1.3);
    ctx.strokeStyle="rgba(77,204,226,.15)";ctx.lineWidth=1;ctx.stroke();
    if(activity==="thinking"&&!reduceMotion){
      const y=40+(seconds*28)%215;
      const scan=ctx.createLinearGradient(0,y-12,0,y+12);
      scan.addColorStop(0,"rgba(66,211,244,0)");
      scan.addColorStop(.5,"rgba(66,211,244,.12)");
      scan.addColorStop(1,"rgba(66,211,244,0)");
      ctx.fillStyle=scan;ctx.fillRect(24,y-12,208,24);
    }
  }

  function faceSurface() {
    // Underlay gives a stable silhouette, rather than an open oval of wires.
    path(outline,"rgba(109,216,225,.31)",1.25,"rgba(6,31,45,.88)");
    for(const f of triangles){
      const a=rotated[f.a],b=rotated[f.b],c=rotated[f.c];
      const ax=b.x-a.x, ay=b.y-a.y, az=b.z-a.z;
      const bx=c.x-a.x, by=c.y-a.y, bz=c.z-a.z;
      const nx=ay*bz-az*by,ny=az*bx-ax*bz,nz=ax*by-ay*bx;
      const norm=Math.hypot(nx,ny,nz)||1;
      f.light=clamp(.20 + (.20*nx+.33*ny+.70*Math.abs(nz))/norm,.11,1);
      f.z=(a.z+b.z+c.z)/3;
    }
    triangles.sort((a,b)=>a.z-b.z);
    for(const f of triangles){
      const a=projected[f.a],b=projected[f.b],c=projected[f.c],l=f.light;
      const red=Math.round(7+13*l),green=Math.round(26+46*l),blue=Math.round(37+53*l);
      ctx.beginPath();ctx.moveTo(a.x,a.y);ctx.lineTo(b.x,b.y);ctx.lineTo(c.x,c.y);ctx.closePath();
      ctx.fillStyle="rgb("+red+","+green+","+blue+")";ctx.fill();
      ctx.lineWidth=.36;ctx.strokeStyle="rgba(79,187,203,"+(.045+.12*l)+")";ctx.stroke();
    }
    // Stronger anatomy, softer internal triangulation.
    path([234,132,58,172,136,150,149,176,148,152,377,400,378,379,365,397,288,361,454],
      "rgba(121,223,222,.40)",1.0);
    path([116,123,147,187,205],"rgba(50,153,175,.20)",.7);
    path([345,352,376,411,425],"rgba(50,153,175,.20)",.7);
  }

  function eye(e, xShift, yShift) {
    const a=projected[e.outer], b=projected[e.inner];
    const top=projected[e.upper], bottom=projected[e.lower];
    const cx=(a.x+b.x)/2, cy=(top.y+bottom.y)/2;
    const opening=clamp(Math.abs(bottom.y-top.y),.5,18);
    const width=Math.abs(b.x-a.x);
    ctx.beginPath();ctx.moveTo(a.x,a.y);
    ctx.quadraticCurveTo(top.x,top.y,b.x,b.y);
    ctx.quadraticCurveTo(bottom.x,bottom.y,a.x,a.y);ctx.closePath();
    ctx.fillStyle="rgba(1,13,24,.95)";ctx.fill();
    ctx.lineWidth=1.25;ctx.strokeStyle="rgba(118,231,227,.85)";ctx.stroke();
    if(opening>2.5) {
      const dx=clamp(xShift*width*.18,-width*.16,width*.16);
      const dy=clamp(yShift*opening*.2,-opening*.16,opening*.16);
      glow(cx+dx,cy+dy,Math.max(1.3,Math.min(2.8,opening*.25)),"#b9fff0",12);
      glow(cx+dx,cy+dy,1,"#e9ffff",4);
    }
    path(e.brow,"rgba(113,223,229,.56)",1.35);
  }

  function facialFeatures(seconds, e) {
    for(const feature of eyes) eye(feature,pose.gazeX,pose.gazeY);
    path([168,6,197,1,4,5],"rgba(115,217,224,.38)",1);
    path([98,97,2,326,327],"rgba(85,190,200,.35)",.75);
    // Lip opening uses INNER vermilion landmarks (13 and 14), not the chin.
    // Anchor corners on IDs 61/291 so the mouth cannot float off the mesh.
    const a=projected[61], b=projected[291], upper=projected[13], lower=projected[14];
    const mid=(upper.y+lower.y)/2;
    const aperture=clamp(e.speech*pose.jaw*2.15,0,1);
    const topY=lerp(mid,upper.y,aperture*.9);
    const bottomY=lerp(mid,lower.y,aperture*.9);
    ctx.beginPath();ctx.moveTo(a.x,a.y);
    ctx.quadraticCurveTo(upper.x,topY,b.x,b.y);
    ctx.quadraticCurveTo(lower.x,bottomY,a.x,a.y);
    if(aperture>.09){
      ctx.closePath();ctx.fillStyle="rgba(2,11,18,.94)";ctx.fill();
    }
    ctx.lineWidth=aperture>.09?1.25:1.55;
    ctx.strokeStyle="rgba(123,232,219,.82)";ctx.stroke();
    if(aperture>.28){
      const shineY=(topY+bottomY)/2;
      ctx.beginPath();ctx.moveTo(upper.x-9,shineY);
      ctx.lineTo(upper.x+9,shineY);
      ctx.lineWidth=.85;ctx.strokeStyle="rgba(138,244,241,.22)";ctx.stroke();
    }
    if(activity==="listening"){
      ctx.beginPath();ctx.ellipse(128,150,108,134,0,0,Math.PI*2);
      ctx.strokeStyle="rgba(99,235,219,.28)";ctx.lineWidth=.9;ctx.stroke();
    }
  }

  function render(time) {
    const seconds=time/1000;
    const dt=lastTime ? clamp((time-lastTime)/1000,.001,.06) : .016;
    lastTime=time;
    const e=expression(seconds,dt);
    buildFace(seconds,e);
    backdrop(seconds);faceSurface();facialFeatures(seconds,e);
    window.jarvisFaceMeshReady=true;
  }

  const dpr=clamp(window.devicePixelRatio||1,1,2);
  canvas.width=W*dpr;canvas.height=H*dpr;
  ctx.setTransform(dpr,0,0,dpr,0,0);
  window.jarvisFaceMeshInfo={vertices:V,triangles:triangles.length,renderer:"Canvas2D",expressions:true};
  window.jarvisSetFaceActivity=(state)=>{
    activity=Object.prototype.hasOwnProperty.call(presets,state)?state:"ready";
    target=presets[activity];
    if(reduceMotion) render(0);
  };
  function tick(time){
    framePending=false;
    if(!active||document.hidden)return;
    if(time-frameTime>=32){frameTime=time;render(time);}
    requestFrame();
  }
  function requestFrame(){
    if(!framePending&&!reduceMotion&&!document.hidden){
      framePending=true;
      requestAnimationFrame(tick);
    }
  }
  document.addEventListener("visibilitychange",()=>{
    active=!document.hidden;
    if(active){lastTime=0;requestFrame();}
  });
  render(0);
  requestFrame();
})();