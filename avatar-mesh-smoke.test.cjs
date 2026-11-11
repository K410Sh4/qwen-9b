/* Offline smoke test for the embedded facial renderer (Node CI only). */
"use strict";
const fs = require("node:fs");
const path = require("node:path");
const vm = require("node:vm");
const assert = require("node:assert/strict");

const mesh = JSON.parse(fs.readFileSync(path.join(__dirname, "avatar-mesh.json"), "utf8"));
assert.equal(mesh.positions.length, 468 * 3);
assert.equal(mesh.triangles.length, 898 * 3);
let fills = 0;
let geometryCalls = 0;
const numberMethods = new Set(["moveTo","lineTo","arc","ellipse","quadraticCurveTo","fillRect","clearRect","setTransform"]);
const context = new Proxy({}, {
    get(target, prop) {
        if (prop === "createRadialGradient" || prop === "createLinearGradient") {
            return (...args) => {
                assert(args.every(Number.isFinite), "gradient with nonfinite coords");
                return { addColorStop(position) { assert(Number.isFinite(position)); } };
            };
        }
        return (...args) => {
            if (numberMethods.has(prop)) {
                assert(args.every(Number.isFinite), "nonfinite canvas coordinates in " + prop);
                geometryCalls++;
            }
            if (prop === "fill") fills++;
        };
    },
    set(target,prop,value){target[prop]=value;return true;}
});
const canvas = {
    width:0, height:0,
    getContext(kind) { assert.equal(kind,"2d");return context; }
};
const listeners = {};
const doc = {
    hidden:false,
    getElementById(id) { assert.equal(id,"faceMesh");return canvas; },
    addEventListener(name,callback) { listeners[name]=callback; }
};
const win = {
    jarvisMeshGeometry:mesh,
    devicePixelRatio:1,
    matchMedia() {return {matches:true};}
};
const sandbox = {
    window:win,document:doc,
    matchMedia:win.matchMedia,
    requestAnimationFrame() {throw new Error("animated frame in reduced motion");},
    Float32Array,Uint16Array,Array,Math,Set
};
const source = fs.readFileSync(path.join(__dirname,"avatar-mesh-renderer.js"),"utf8");
vm.runInNewContext(source,sandbox,{timeout:6000,filename:"avatar-mesh-renderer.js"});
assert.equal(win.jarvisFaceMeshReady,true);
assert.equal(win.jarvisFaceMeshInfo.vertices,468);
assert.equal(win.jarvisFaceMeshInfo.triangles,898);
assert.equal(win.jarvisFaceMeshInfo.expressions,true);
assert.ok(fills >= 898,"initial frame has no rendered mesh");
for(const activity of ["ready","listening","thinking","speaking","ready"]){
    const before = fills;
    win.jarvisSetFaceActivity(activity);
    assert(fills-before>=898,"no frame rendered for " + activity);
}
assert.ok(geometryCalls > 2000);
console.log("Face mesh: 468 vertices / 898 triangles, 4 expressions, finite coordinates OK");
