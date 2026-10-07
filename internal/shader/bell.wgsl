struct Uniforms { words: array<vec4<u32>,16>, }
@group(0) @binding(0) var<uniform> u: Uniforms;
@group(0) @binding(1) var pixels: texture_storage_2d<rgba8unorm,write>;
@group(0) @binding(2) var<storage,read> samples: array<vec4<f32>>;
fn word(i:u32)->u32{return u.words[i/4u][i%4u];}
fn val(i:u32)->f32{return bitcast<f32>(word(i));}
fn enabled(i:u32)->bool{return (word(11u)&(1u<<i))!=0u;}
struct Bell {weight:f32, mu:vec2<f32>, sigma:vec2<f32>, rho:f32, color:vec3<f32>,}
fn bell(i:u32)->Bell{let o=16u+i*6u;let rgb=word(o+5u);return Bell(val(o),vec2<f32>(val(o+1u),val(o+2u)),unpack2x16float(word(o+3u)),val(o+4u),vec3<f32>(f32((rgb>>16u)&255u),f32((rgb>>8u)&255u),f32(rgb&255u))/255.0);}
fn gaussian2(p:vec2<f32>,c:Bell)->f32{let q=(p-c.mu)/c.sigma;let v=1.0-c.rho*c.rho;return exp(-(dot(q,q)-2.0*c.rho*q.x*q.y)/(2.0*v))/(6.2831853*c.sigma.x*c.sigma.y*sqrt(v));}
fn mixture(p:vec2<f32>)->f32{var z=0.0;for(var i=0u;i<word(15u);i++){let c=bell(i);z+=c.weight*gaussian2(p,c);}return z;}
fn elevation(p:vec2<f32>)->f32{var denom=1.0;if(enabled(5u)){denom=max(val(14u),0.000001);}return mixture(p)*val(13u)/denom;}
fn palette(t:f32)->vec3<f32>{return mix(vec3<f32>(0.13,0.21,0.32),vec3<f32>(0.98,0.82,0.59),pow(clamp(t,0.0,1.0),0.6));}
fn ellipse_point(c:Bell,theta:f32,k:f32)->vec2<f32>{return c.mu+k*c.sigma*vec2<f32>(cos(theta),c.rho*cos(theta)+sqrt(1.0-c.rho*c.rho)*sin(theta));}
fn ellipse_distance(p:vec2<f32>,c:Bell)->f32{let q=(p-c.mu)/c.sigma;return sqrt(max(0.0,(dot(q,q)-2.0*c.rho*q.x*q.y)/(1.0-c.rho*c.rho)));}
fn marginal(x:f32,axis:u32)->f32{var z=0.0;for(var i=0u;i<word(15u);i++){let c=bell(i);var mu=c.mu.x;var sigma=c.sigma.x;if(axis==1u){mu=c.mu.y;sigma=c.sigma.y;}let q=(x-mu)/sigma;z+=c.weight*exp(-0.5*q*q)/(2.5066283*sigma);}return z;}
fn intersect_box(ro:vec3<f32>,rd:vec3<f32>,lo:vec3<f32>,hi:vec3<f32>)->vec2<f32>{let a=(lo-ro)/rd;let b=(hi-ro)/rd;let near=min(a,b);let far=max(a,b);return vec2<f32>(max(max(near.x,near.y),near.z),min(min(far.x,far.y),far.z));}
@compute @workgroup_size(8,8)
fn main(@builtin(global_invocation_id) id:vec3<u32>){
 let wh=vec2<u32>(word(8u),word(9u));if(any(id.xy>=wh)){return;}
 let eye=vec3<f32>(val(0u),val(1u),val(2u));let lookAt=vec3<f32>(val(4u),val(5u),val(6u));
 let forward=normalize(lookAt-eye);let right=normalize(cross(forward,vec3<f32>(0.0,0.0,1.0)));let up=cross(right,forward);
 let uv=(vec2<f32>(id.xy)+0.5-vec2<f32>(wh)*0.5)/f32(wh.y);let ray=normalize(forward+(right*uv.x-up*uv.y)*2.0*tan(val(3u)*0.5));
 let extent=val(12u);var peak=val(14u);if(enabled(5u)){peak=1.0;}let maxheight=val(13u)*peak;let bounds=intersect_box(eye,ray,vec3<f32>(-extent,-extent,0.0),vec3<f32>(extent,extent,maxheight+0.01));
 var color=vec3<f32>(0.025,0.035,0.055)+vec3<f32>(0.012)*(1.0-uv.y);var hit=false;var p=vec3<f32>(0.0);var distance=1e6;
 if(bounds.y>max(bounds.x,0.0)){
 var t=max(bounds.x,0.0);let step=(bounds.y-t)/256.0;var previous=t;
 for(var j=0u;j<257u;j++){p=eye+ray*t;if(p.z<=elevation(p.xy)){var a=previous;var b=t;for(var k=0u;k<9u;k++){let mid=(a+b)*0.5;let q=eye+ray*mid;if(q.z>elevation(q.xy)){a=mid;}else{b=mid;}}distance=(a+b)*0.5;p=eye+ray*distance;hit=true;break;}previous=t;t+=step;}
 }
 if(hit){let eps=0.002*extent;let n=normalize(vec3<f32>(elevation(p.xy-vec2<f32>(eps,0.0))-elevation(p.xy+vec2<f32>(eps,0.0)),elevation(p.xy-vec2<f32>(0.0,eps))-elevation(p.xy+vec2<f32>(0.0,eps)),2.0*eps));let z=mixture(p.xy)/max(val(14u),0.000001);color=palette(z)*(0.55+0.45*max(0.0,dot(n,normalize(vec3<f32>(-0.5,-0.7,1.5)))));if(enabled(4u)&&(abs(z-0.5)<0.008||abs(z-0.1)<0.006)){color=vec3<f32>(0.97,0.87,0.68);}}
 // Ground grid and per-component covariance ellipses share the ray pass.
 let ground=-eye.z/ray.z;let gp=(eye+ray*ground).xy;
 if(ground>0.0&&all(abs(gp)<vec2<f32>(extent))){var groundColor=vec3<f32>(0.055,0.075,0.105);var line=false;
 if(enabled(0u)){let g=abs(gp-round(gp));if(min(g.x,g.y)<0.018){groundColor=vec3<f32>(0.16,0.22,0.28);line=true;}if(min(abs(gp.x),abs(gp.y))<0.025){groundColor=vec3<f32>(0.27,0.34,0.41);line=true;}}
 if(enabled(1u)){for(var i=0u;i<word(15u);i++){let c=bell(i);let d=ellipse_distance(gp,c);if(abs(d-1.0)<0.035||abs(d-2.0)<0.035){groundColor=c.color;line=true;}}}
 if(!hit&&enabled(0u)){color=groundColor;}else if(line){color=mix(color,groundColor,select(0.8,0.35,hit));}
 }
 // Analytic marginals on the far x/y walls.
 if(enabled(2u)){for(var axis=0u;axis<2u;axis++){var ea=eye.x;var ra=ray.x;if(axis==1u){ea=eye.y;ra=ray.y;}var side=extent;if(ea>0.0){side=-extent;}let t=(side-ea)/ra;let q=eye+ray*t;let other=1u-axis;var qo=q.y;if(axis==1u){qo=q.x;}let h=marginal(qo,other)*val(13u);if(t>0.0&&abs(qo)<extent&&q.z>=0.0&&abs(q.z-h)<0.035*extent){color=mix(color,vec3<f32>(0.6,0.7,0.88),select(0.95,0.35,t>distance));}}}
 // Seeded cloud is uploaded once per scene edit. Project points rather than
 // introducing frame-dependent random samples or a second render pipeline.
 if(enabled(3u)){let tanf=tan(val(3u)*0.5);for(var i=0u;i<arrayLength(&samples);i++){let sp=samples[i];let point=vec3<f32>(sp.xy,0.04);let rel=point-eye;let depth=dot(rel,forward);if(depth>0.0){let screen=vec2<f32>(dot(rel,right),-dot(rel,up))/(2.0*tanf*depth);let delta=(screen-uv)*f32(wh.y);if(dot(delta,delta)<1.8){let rgb=u32(sp.w);let tint=vec3<f32>(f32((rgb>>16u)&255u),f32((rgb>>8u)&255u),f32(rgb&255u))/255.0;color=mix(color,tint,0.65+0.15*sin(val(7u)+f32(i)));}}}}
 textureStore(pixels,vec2<i32>(id.xy),vec4<f32>(clamp(color,vec3<f32>(0.0),vec3<f32>(1.0)),1.0));
}
