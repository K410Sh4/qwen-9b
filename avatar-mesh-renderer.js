/* Jarvis procedural 3D face, Canvas2D. Canonical geometry: MediaPipe (Apache 2.0). */
(function () {
  "use strict";
  const mesh = window.jarvisMeshGeometry;
  const canvas = document.getElementById("faceMesh");
  if (!canvas || !mesh || !Array.isArray(mesh.positions) || mesh.positions.length !== 1404 ||
      !Array.isArray(mesh.triangles) || mesh.triangles.length !== 2694) {
    throw new Error("Geometria facial canônica ausente");
  }
  const ctx = canvas.getContext("2d", {alpha:true});
  if (!ctx) throw new Error("Canvas 2D indisponível");
  const positions = Float32Array.from(mesh.positions);
  const triangles = Uint16Array.from(mesh.triangles);
  const V = positions.length / 3;
  const W = 256, H = 304;
  const points = Array.from({length:V}, () => ({x:0,y:0,z:0}));
  const rotated = Array.from({length:V}, () => ({x:0,y:0,z:0}));
  const faces = [];
  for (let k=0; k<triangles.length; k+=3) {
    faces.push({a:triangles[k],b:triangles[k+1],c:triangles[k+2],z:0,shade:0});
  }
  let activity = "ready", lastFrame = -1;
  const reduceMotion = window.matchMedia && matchMedia("(prefers-reduced-motion: reduce)").matches;
  let firstFrame = false;

  function projectVertices(seconds) {
    const talking = activity === "speaking" && !reduceMotion;
    const mouth = talking ? 0.45 + 0.55 * Math.abs(Math.sin(seconds * 10.7)) : 0;
    const yaw = reduceMotion ? 0 : Math.sin(seconds*.38)*.105 + (activity==="thinking"?.065:0);
    const pitch = reduceMotion ? 0 : Math.sin(seconds*.29)*.038;
    const cy = Math.cos(yaw), sy = Math.sin(yaw), cp = Math.cos(pitch), sp = Math.sin(pitch);
    for(let i=0;i<V;i++){
      let x=positions[3*i], y=positions[3*i+1], z=positions[3*i+2];
      // Subtle jaw deformation: the canonical topology is never replaced.
      if(y<-4.0){
        const falloff = Math.max(0,Math.min(1,(y+4.0)/-3.9));
        y -= mouth*(Math.abs(x)<3.3 ? .45 : .12)*falloff;
      }
      const xr=x*cy+z*sy, zr=z*cy-x*sy;
      const yr=y*cp-zr*sp, zz=zr*cp+y*sp;
      const f=27/(27-zz), scale=11.8*f;
      const a=rotated[i], p=points[i];
      a.x=xr;a.y=yr;a.z=zz;
      p.x=W*.5+xr*scale;p.y=141-yr*scale;p.z=zz;
    }
    return mouth;
  }

  function drawPath(indices, fill, stroke, width) {
    const first=points[indices[0]];
    ctx.beginPath();ctx.moveTo(first.x,first.y);
    for(let i=1;i<indices.length;i++){
      const p=points[indices[i]];ctx.lineTo(p.x,p.y);
    }
    if(fill){ctx.closePath();ctx.fillStyle=fill;ctx.fill();}
    if(stroke){ctx.lineWidth=width||1.2;ctx.strokeStyle=stroke;ctx.stroke();}
  }

  function glowPoint(p,rgb,radius){
    ctx.save();ctx.shadowColor=rgb;ctx.shadowBlur=16;ctx.fillStyle=rgb;
    ctx.beginPath();ctx.arc(p.x,p.y,radius,0,Math.PI*2);ctx.fill();ctx.restore();
  }

  function drawFeatures(seconds,mouth){
    const blink=reduceMotion ? 0 : (seconds%5.4>5.22 ? 1 : 0);
    const listening=activity==="listening";
    // Canonical eye-contour, eyebrow and lip vertex IDs.
    for(const eye of [
      {loop:[33,160,158,133,153,144], brow:[70,63,105,66,107], top:159,bottom:145,ends:[33,133]},
      {loop:[362,385,387,263,373,380],brow:[336,296,334,293,300],top:386,bottom:374,ends:[362,263]}
    ]){
      if(blink){
        drawPath(eye.ends,null,"rgba(123,249,249,.86)",1.6);
      } else {
        drawPath(eye.loop,"rgba(0,10,23,.92)","rgba(108,244,249,.9)",1.15);
        const p1=points[eye.top],p2=points[eye.bottom];
        const eyePoint={x:(p1.x+p2.x)/2,y:(p1.y+p2.y)/2};
        glowPoint(eyePoint,listening?"#d0ffff":"#7affec",listening?3.7:2.65);
      }
      drawPath(eye.brow,null,"rgba(114,231,238,.58)",1.2);
    }
    drawPath([168,6,197,1,4,5],null,"rgba(106,208,219,.52)",1.0);
    drawPath([61,40,37,0,267,270,291,321,314,17,84,91],
       mouth>.2?"rgba(1,13,22,.95)":"rgba(4,28,39,.8)",
       "rgba(120,234,221,.86)",mouth>.2?1.55:1.05);
    if(mouth>.22){
      const p=points[13],q=points[14];
      ctx.strokeStyle="rgba(139,248,248,.64)";ctx.lineWidth=1.1;
      ctx.beginPath();ctx.moveTo(p.x-10,p.y+1);ctx.quadraticCurveTo((p.x+q.x)/2,q.y+2,q.x+10,q.y+1);ctx.stroke();
    }
    glowPoint(points[4],"rgba(78,199,226,.58)",1.2);
  }

  function render(time){
    const seconds=time/1000;
    ctx.clearRect(0,0,W,H);
    const intensity=activity==="thinking"?.34:activity==="speaking"?.29:.16;
    const halo=ctx.createRadialGradient(128,148,26,128,148,139);
    halo.addColorStop(0,"rgba(5,37,63,"+(intensity+.12)+")");
    halo.addColorStop(.7,"rgba(4,29,51,"+intensity+")");
    halo.addColorStop(1,"rgba(0,0,0,0)");
    ctx.fillStyle=halo;ctx.fillRect(0,0,W,H);
    ctx.save();
    ctx.strokeStyle="rgba(56,190,223,.21)";ctx.lineWidth=.7;
    ctx.beginPath();ctx.ellipse(128,150,101,126,0,-1.25,1.25);ctx.stroke();
    ctx.beginPath();ctx.ellipse(128,150,108,137,0,2.1,4.25);ctx.stroke();
    ctx.restore();

    const mouth=projectVertices(seconds);
    for(const f of faces){
      const a=rotated[f.a],b=rotated[f.b],c=rotated[f.c];
      const ax=b.x-a.x, ay=b.y-a.y,az=b.z-a.z;
      const bx=c.x-a.x, by=c.y-a.y,bz=c.z-a.z;
      let nz=ax*by-ay*bx;
      const nx=ay*bz-az*by,ny=az*bx-ax*bz;
      const norm=Math.sqrt(nx*nx+ny*ny+nz*nz)||1;
      // Canonical face triangles may have reversed winding across boundaries;
      // use absolute frontal illumination but preserve local depth and faceting.
      f.shade=Math.max(.09,Math.min(1,.48+(.16*nx+.31*ny+.86*Math.abs(nz))/norm));
      f.z=(a.z+b.z+c.z)/3;
    }
    faces.sort((a,b)=>a.z-b.z);
    for(const f of faces){
      const a=points[f.a],b=points[f.b],c=points[f.c],l=f.shade;
      const r=Math.round(5+15*l),g=Math.round(22+43*l),bb=Math.round(35+55*l);
      ctx.beginPath();ctx.moveTo(a.x,a.y);ctx.lineTo(b.x,b.y);ctx.lineTo(c.x,c.y);ctx.closePath();
      ctx.fillStyle="rgb("+r+","+g+","+bb+")";ctx.fill();
      ctx.lineWidth=.48;
      ctx.strokeStyle="rgba(64,198,218,"+(.16+.26*l)+")";ctx.stroke();
    }
    drawFeatures(seconds,mouth);
    if(activity==="thinking" && !reduceMotion){
      const y=35+(seconds*34)%212;
      const scan=ctx.createLinearGradient(0,y-12,0,y+12);
      scan.addColorStop(0,"rgba(60,216,248,0)");
      scan.addColorStop(.5,"rgba(60,216,248,.16)");
      scan.addColorStop(1,"rgba(60,216,248,0)");
      ctx.fillStyle=scan;ctx.fillRect(33,y-12,190,24);
    }
    if(activity==="listening"){
      ctx.beginPath();ctx.ellipse(128,150,109,136,0,0,Math.PI*2);
      ctx.strokeStyle="rgba(99,242,235,.42)";ctx.lineWidth=1.2;ctx.stroke();
    }
    firstFrame=true;
  }

  // Prefer an offline renderer: no webcam, external scripts or ML runtime.
  const dpr=Math.min(2,Math.max(1,window.devicePixelRatio||1));
  canvas.width=W*dpr;canvas.height=H*dpr;
  ctx.setTransform(dpr,0,0,dpr,0,0);
  render(0);
  window.jarvisFaceMeshReady=firstFrame;
  window.jarvisFaceMeshInfo={vertices:V,triangles:faces.length,renderer:"Canvas2D"};
  window.jarvisSetFaceActivity=function(next){activity=next||"ready";};
  function animate(now){
    if(!reduceMotion && now-lastFrame>=33){
      lastFrame=now;render(now);
    }
    if(!document.hidden && !reduceMotion) requestAnimationFrame(animate);
  }
  function resume(){
    if(!document.hidden && !reduceMotion){lastFrame=-1;requestAnimationFrame(animate);}
  }
  if(!reduceMotion) {
    document.addEventListener("visibilitychange",resume);
    requestAnimationFrame(animate);
  }
})();
