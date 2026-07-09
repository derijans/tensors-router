(function(){let e=document.createElement(`link`).relList;if(e&&e.supports&&e.supports(`modulepreload`))return;for(let e of document.querySelectorAll(`link[rel="modulepreload"]`))n(e);new MutationObserver(e=>{for(let t of e)if(t.type===`childList`)for(let e of t.addedNodes)e.tagName===`LINK`&&e.rel===`modulepreload`&&n(e)}).observe(document,{childList:!0,subtree:!0});function t(e){let t={};return e.integrity&&(t.integrity=e.integrity),e.referrerPolicy&&(t.referrerPolicy=e.referrerPolicy),e.crossOrigin===`use-credentials`?t.credentials=`include`:e.crossOrigin===`anonymous`?t.credentials=`omit`:t.credentials=`same-origin`,t}function n(e){if(e.ep)return;e.ep=!0;let n=t(e);fetch(e.href,n)}})();function e(){return{text:null,image:null,embeddings:null,voice:null,music:null}}function t(){return{text:``,image:``,embeddings:``,voice:``,music:``}}function n(){return{text:{},image:{},embeddings:{},voice:{},music:{}}}var r={csrf:``,inventory:null,router:null,benchmark:{modelKey:``,type:`general`,sections:[`runtime`,`llm`,`embed`,`image`,`voice`,`music`],record:null,running:!1,error:``},analytics:{query:{period:`24h`},data:null,loading:!1,error:``},webuis:{data:null,filter:``,loading:!1,error:``,action:``},activeTab:`router`,activeCookMode:`quick`,activePalette:`configs`,simpleCook:{nodeID:``,configID:``,fields:{},cleanFields:{},mode:`edit`,fieldFilter:``,openSections:[],sidebar:null},constructor:{lanes:e(),targetNodes:t(),laneOptions:n(),backendMode:`kobold`,backendTouched:!1,options:{},fieldEditor:null,fieldPresets:[],showUsedAll:!1,showOptionsAll:!1},palettePayloads:{}};function i(e){return typeof e!=`object`||!e||Array.isArray(e)?!1:Object.values(e).every(a)}function a(e){if(e===null)return!0;switch(typeof e){case`boolean`:case`number`:case`string`:return!0;case`object`:return Array.isArray(e)?e.every(a):i(e);default:return!1}}function o(e){return typeof e==`object`&&e&&!Array.isArray(e)?e:null}async function s(e,t={}){let n=new Headers(t.headers);t.body&&!n.has(`Content-Type`)&&n.set(`Content-Type`,`application/json`),r.csrf&&t.method&&t.method!==`GET`&&n.set(`X-CSRF-Token`,r.csrf);let i=await fetch(e,{...t,headers:n}),a=await i.text(),o=Ce(a);if(!i.ok)throw Te(we(o,a,i.statusText),o);return o}function c(){return s(`/api/session`)}function ee(e){return s(`/api/login`,{method:`POST`,body:JSON.stringify({token:e})})}function te(){return s(`/api/logout`,{method:`POST`})}function ne(){return s(`/api/router/status`)}function re(){return s(`/api/router/launch`,{method:`POST`})}function ie(){return s(`/api/router/restart`,{method:`POST`})}function ae(){return s(`/api/router/shutdown`,{method:`POST`})}function oe(){return s(`/api/router/force-kill`,{method:`POST`})}function se(){return s(`/api/inventory`)}function ce(){return s(`/api/webuis`)}function le(e){return s(`/api/webuis/session`,{method:`POST`,body:JSON.stringify(e)})}function ue(e){return s(`/api/webuis/load`,{method:`POST`,body:JSON.stringify(e)})}function de(e,t){let n=new URLSearchParams({model_id:t});return e&&n.set(`node_id`,e),s(`/api/benchmarks?${n.toString()}`)}function fe(e){return s(`/api/benchmarks/run`,{method:`POST`,body:JSON.stringify(e)})}function pe(e){let t=new URLSearchParams({period:e.period});return e.node_id&&t.set(`node_id`,e.node_id),e.model_id&&t.set(`model_id`,e.model_id),e.section&&t.set(`section`,e.section),s(`/api/analytics?${t.toString()}`)}function me(e){return s(`/api/load`,{method:`POST`,body:JSON.stringify(e)})}function he(e){return s(`/api/cook/preview`,{method:`POST`,body:JSON.stringify(e)})}function ge(e){return s(`/api/cook/apply`,{method:`POST`,body:JSON.stringify(e)})}function _e(e){return s(`/api/cook/${encodeURIComponent(e)}`,{method:`DELETE`})}function ve(e){return s(`/api/config-file/preview`,{method:`POST`,body:JSON.stringify(e)})}function ye(e){return s(`/api/config-file/apply`,{method:`POST`,body:JSON.stringify(e)})}function be(e){return s(`/api/config-file`,{method:`DELETE`,body:JSON.stringify(e)})}function xe(e){if(Se(e)){let t=Ee(o(e.data)?.validation);return t?{error:e.message,validation:t}:{error:e.message}}return{error:e instanceof Error?e.message:String(e)}}function Se(e){return e instanceof Error&&`data`in e}function Ce(e){if(!e)return null;try{return JSON.parse(e)}catch{return{raw:e}}}function we(e,t,n){let r=o(e);if(typeof r?.error==`string`)return r.error;let i=o(r?.error);return typeof i?.message==`string`?i.message:t||n}function Te(e,t){let n=Error(e);return n.data=t,n}function Ee(e){if(!Array.isArray(e))return null;let t=e.filter(De);return t.length>0?t:null}function De(e){let t=o(e);return typeof t?.severity==`string`&&typeof t.code==`string`&&typeof t.message==`string`}var l=[`runtime`,`llm`,`embed`,`image`,`voice`,`music`];function Oe(e){let t=e.benchmark?.latest;if(!t)return`none`;let n=ke(t,`tokens_per_second`);return n?.value?`${t.status} ${n.value.toFixed(1)} tok/s`:`${t.status} ${t.duration_ms||0}ms`}function ke(e,t){return e.metrics?.find(e=>e.name===t)??null}function u(e){let t={"&":`&amp;`,"<":`&lt;`,">":`&gt;`,'"':`&quot;`,"'":`&#39;`};return Ve(e).replace(/[&<>"']/g,e=>t[e]??e)}function d(e){return u(e).replace(/`/g,`&#96;`)}function f(e,t){return`
    <div class="status-item">
      <div class="status-label">${u(e)}</div>
      <div class="status-value">${u(t)}</div>
    </div>
  `}function p(e,t){let n=Ve(e).trim();return n?`<span class="chip ${d(t)}">${u(n)}</span>`:``}function Ae(e){return`
    <div class="issue ${e.severity===`error`?`error`:``}">
      <strong>${u(e.severity)} / ${u(e.code)}</strong>
      <span>${u(e.message)}</span>
    </div>
  `}function m(e,t,n,r){return{severity:e,code:t,message:n,field:r}}function je(e){switch(e){case`image`:return`magenta`;case`embeddings`:return`lime`;case`voice`:return`amber`;case`music`:return`violet`;default:return`cyan`}}function Me(e){let t=[];return e.has_llm&&t.push(`llm`),e.has_image&&t.push(`image`),e.has_embeddings&&t.push(`embeddings`),e.has_multimodal&&t.push(`multimodal`),e.has_voice&&t.push(`voice`),e.has_music&&t.push(`music`),t.join(`, `)||`none`}function Ne(e){let t=Object.keys(e??{}).length;return t?`${t} filled`:`none`}function h(e){return e.roles??[e.role||`unknown`]}function Pe(e,t){return e.some(e=>e.kind===t)}function Fe(e){let t=String(e).toLowerCase();return[`gpulayers`,`tensor_split`,`maingpu`,`usecuda`,`usecublas`,`embeddingsgpu`,`sdclipgpu`,`sdflashattention`].includes(e)||t.includes(`gpu`)||t.includes(`cuda`)}function Ie(e){return typeof e==`boolean`?e:typeof e==`number`?e!==0:typeof e==`string`?e.trim()!==``:e!=null}function Le(e){if(typeof e==`number`)return e;if(typeof e==`string`){let t=Number.parseInt(e,10);return Number.isFinite(t)?t:0}return 0}function g(e){return typeof e==`string`?e:e===void 0?``:JSON.stringify(e)??``}function _(e){return typeof e==`string`?e:e===void 0?``:JSON.stringify(e)??``}function Re(e,t){let n=t.trim();switch(e?.value_type){case`bool`:return n===`true`||n===`1`||n===`yes`;case`number`:{let e=Number(n);return Number.isFinite(e)?e:0}case`json`:if(!n)return{};try{let e=JSON.parse(n);return a(e)?e:t}catch{return t}default:return t}}function v(e){return e==null?!0:typeof e==`string`?e.trim()===``:Array.isArray(e)?e.length===0||e.every(v):typeof e==`object`?Object.keys(e).length===0:!1}function ze(e){return typeof e==`string`?e.trim():JSON.stringify(e)??``}function Be(e){return e<1024?`${e} B`:e<1024*1024?`${(e/1024).toFixed(1)} KB`:e<1024*1024*1024?`${(e/1024/1024).toFixed(1)} MB`:`${(e/1024/1024/1024).toFixed(1)} GB`}function Ve(e){return e==null?``:typeof e==`string`?e:typeof e==`number`||typeof e==`boolean`||typeof e==`bigint`?e.toString():JSON.stringify(e)??``}function He(){return(r.inventory?.nodes??[]).flatMap(e=>e.models??[])}function Ue(){return(r.inventory?.nodes??[]).flatMap(e=>e.files??[])}function y(){return[...r.inventory?.option_catalog??[],...r.inventory?.observed_options??[]]}function b(e){return y().find(t=>t.key===e)}function x(e){return(r.inventory?.nodes??[]).find(t=>t.node_id===e)}function We(e){let t=r.inventory?.models?.length?r.inventory.models:He();return e?t.filter(t=>JSON.stringify(t).toLowerCase().includes(e)):t}function Ge(e){let t=Ue();return e?t.filter(t=>JSON.stringify(t).toLowerCase().includes(e)):t}function Ke(e){let t=new Map;for(let n of e){let e=n.node_id||r.inventory?.node_id||`local`,i=t.get(e)??[];i.push(n),t.set(e,i)}return t}function qe(e,t,n){let r=[];for(let[i,a]of Object.entries(Je(e,t,n))){if(!et(i))continue;let e=Le(a);e>0&&r.push({key:i,value:e})}return r.sort((e,t)=>e.key.localeCompare(t.key))}function Je(e,t,n){let i={};for(let n of t){let t=r.constructor.lanes[n.kind],a=r.constructor.targetNodes[n.kind]||t?.component.node_id||``;!t||(a||``)!==(e||``)||(Object.assign(i,t.model?.options??{}),Object.assign(i,r.constructor.laneOptions[n.kind]??{}))}return Object.assign(i,n),i}function Ye(){let e={};for(let t of it())Object.assign(e,t.model?.options??{}),Object.assign(e,r.constructor.laneOptions[t.lane]??{});return Object.assign(e,r.constructor.options),e}function Xe(e){let t=e.model?.options??{},n=[];for(let e of[`model_param`,`model`,`sdmodel`,`embeddingsmodel`,`mmproj`,`sdvae`,`sdt5xxl`,`sdclipl`,`sdclipg`,`sdupscaler`,`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`,`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]){let r=t[e];if(typeof r==`string`&&r.trim())n.push(`${e}: ${r}`);else if(Array.isArray(r))for(let t of r)typeof t==`string`&&t.trim()&&n.push(`${e}: ${t}`)}return e.file?.path&&n.push(`file: ${e.file.path}`),n}function Ze(){return He().flatMap(e=>{let t=[];return e.has_llm&&t.push(S(`text`,e)),e.has_image&&t.push(S(`image`,e)),e.has_embeddings&&t.push(S(`embeddings`,e)),e.has_voice&&t.push(S(`voice`,e)),e.has_music&&t.push(S(`music`,e)),t})}function Qe(){return Ue().flatMap(e=>{let t=[];return h(e).includes(`llm`)&&t.push(C(`text`,e)),h(e).includes(`image`)&&t.push(C(`image`,e)),h(e).includes(`embeddings`)&&t.push(C(`embeddings`,e)),h(e).includes(`voice`)&&t.push(C(`voice`,e)),h(e).includes(`music`)&&t.push(C(`music`,e)),t})}function $e(){return y().map(e=>({title:e.name||e.key,subtitle:e.key,badge:e.lane||`option`,color:e.known?`cyan`:`amber`,meta:[e.value_type||`json`,...e.backends??[],e.native_flag??``,e.known?`known`:`observed`].filter(w),payload:{type:`option`,key:e.key}}))}function et(e){let t=b(e);return t?t.value_type===`number`&&t.key.endsWith(`threads`):String(e||``).trim().toLowerCase().endsWith(`threads`)}function S(e,t){let n=e===`image`?t.public_image_id||t.image_id||t.local_id:t.public_id||t.local_id;return{title:n,subtitle:t.filename||``,badge:e,color:je(e),meta:[t.node_id||``,t.backend_mode||``,rt(t.options)].filter(w),payload:{type:`component`,lane:e,label:n,subtitle:t.filename||``,meta:[t.node_id||``,t.backend_mode||``].filter(w),component:tt(e,t),model:t}}}function C(e,t){return{title:t.basename,subtitle:t.path,badge:e,color:je(e),meta:[t.node_id||``,Be(t.size||0)].filter(w),payload:{type:`component`,lane:e,label:t.basename,subtitle:t.path,meta:[t.node_id||``,`file`].filter(w),component:nt(e,t),file:t}}}function tt(e,t){let n={kind:e,node_id:t.node_id,node_url:t.node_url||``,source:`config`,model_id:t.local_id};return e===`image`&&(n.image_id=t.image_id||``),n}function nt(e,t){return{kind:e,node_id:t.node_id,source:`file`,file_path:t.path}}function rt(e){let t=Object.keys(e??{}).length;return t?`${t} options`:``}function it(){return Object.values(r.constructor.lanes).filter(e=>e!==null)}function w(e){return e.trim()!==``}function at(e,t){return{value:e,label:t}}function T(e,t){let n=document.getElementById(e);if(!(n instanceof t))throw Error(`Expected #${e} to be ${t.name}`);return n}function E(e){return e.target instanceof HTMLElement?e.target:null}function D(e,t,n){if(!(e instanceof Element))return null;let r=e.closest(t);return r instanceof n?r:null}function O(e,t){return Array.from(document.querySelectorAll(e)).filter(e=>e instanceof t)}var k={loginView:T(`loginView`,HTMLElement),appView:T(`appView`,HTMLElement),loginForm:T(`loginForm`,HTMLFormElement),tokenInput:T(`tokenInput`,HTMLInputElement),loginError:T(`loginError`,HTMLElement),logoutButton:T(`logoutButton`,HTMLButtonElement),refreshButton:T(`refreshButton`,HTMLButtonElement),launchButton:T(`launchButton`,HTMLButtonElement),restartButton:T(`restartButton`,HTMLButtonElement),shutdownButton:T(`shutdownButton`,HTMLButtonElement),forceKillButton:T(`forceKillButton`,HTMLButtonElement),routerSummary:T(`routerSummary`,HTMLElement),routerStatus:T(`routerStatus`,HTMLElement),nodeCount:T(`nodeCount`,HTMLElement),nodesGrid:T(`nodesGrid`,HTMLElement),webuiFilterInput:T(`webuiFilterInput`,HTMLInputElement),webuiStatus:T(`webuiStatus`,HTMLElement),webuiGrid:T(`webuiGrid`,HTMLElement),filterInput:T(`filterInput`,HTMLInputElement),modelsActionStatus:T(`modelsActionStatus`,HTMLElement),modelsTable:T(`modelsTable`,HTMLTableSectionElement),filesTable:T(`filesTable`,HTMLTableSectionElement),benchmarkModelSelect:T(`benchmarkModelSelect`,HTMLSelectElement),benchmarkTypeSelect:T(`benchmarkTypeSelect`,HTMLSelectElement),benchmarkAllSections:T(`benchmarkAllSections`,HTMLInputElement),benchmarkSections:T(`benchmarkSections`,HTMLElement),runBenchmarkButton:T(`runBenchmarkButton`,HTMLButtonElement),benchmarkLatest:T(`benchmarkLatest`,HTMLElement),benchmarkHistory:T(`benchmarkHistory`,HTMLElement),analyticsPeriodSelect:T(`analyticsPeriodSelect`,HTMLSelectElement),analyticsNodeSelect:T(`analyticsNodeSelect`,HTMLSelectElement),analyticsModelSelect:T(`analyticsModelSelect`,HTMLSelectElement),analyticsSectionSelect:T(`analyticsSectionSelect`,HTMLSelectElement),analyticsRefreshButton:T(`analyticsRefreshButton`,HTMLButtonElement),analyticsStatus:T(`analyticsStatus`,HTMLElement),analyticsSummary:T(`analyticsSummary`,HTMLElement),analyticsTimeline:T(`analyticsTimeline`,HTMLElement),analyticsSections:T(`analyticsSections`,HTMLElement),analyticsModelsTable:T(`analyticsModelsTable`,HTMLTableSectionElement),analyticsNodesTable:T(`analyticsNodesTable`,HTMLTableSectionElement),analyticsRecentTable:T(`analyticsRecentTable`,HTMLTableSectionElement),analyticsNodeErrors:T(`analyticsNodeErrors`,HTMLElement),cookForm:T(`cookForm`,HTMLFormElement),cookIdInput:T(`cookIdInput`,HTMLInputElement),overwriteInput:T(`overwriteInput`,HTMLInputElement),simpleNodeSelect:T(`simpleNodeSelect`,HTMLSelectElement),simpleConfigSelect:T(`simpleConfigSelect`,HTMLSelectElement),simpleFieldFilter:T(`simpleFieldFilter`,HTMLInputElement),simpleAddFieldSelect:T(`simpleAddFieldSelect`,HTMLSelectElement),simpleAddFieldButton:T(`simpleAddFieldButton`,HTMLButtonElement),simpleNewButton:T(`simpleNewButton`,HTMLButtonElement),simpleCopyButton:T(`simpleCopyButton`,HTMLButtonElement),simpleDeleteButton:T(`simpleDeleteButton`,HTMLButtonElement),simpleConfigEditor:T(`simpleConfigEditor`,HTMLElement),simpleFieldSidebar:T(`simpleFieldSidebar`,HTMLElement),previewButton:T(`previewButton`,HTMLButtonElement),cookOutput:T(`cookOutput`,HTMLPreElement),recipeCount:T(`recipeCount`,HTMLElement),recipesList:T(`recipesList`,HTMLElement),advancedBackendSelect:T(`advancedBackendSelect`,HTMLSelectElement),advancedCookIdInput:T(`advancedCookIdInput`,HTMLInputElement),constructorFilterInput:T(`constructorFilterInput`,HTMLInputElement),clearConstructorButton:T(`clearConstructorButton`,HTMLButtonElement),advancedPreviewButton:T(`advancedPreviewButton`,HTMLButtonElement),advancedApplyButton:T(`advancedApplyButton`,HTMLButtonElement),paletteList:T(`paletteList`,HTMLElement),constructorLanes:T(`constructorLanes`,HTMLElement),validationList:T(`validationList`,HTMLElement),usedModelsList:T(`usedModelsList`,HTMLElement),selectedOptionsList:T(`selectedOptionsList`,HTMLElement),constructorFieldDialog:T(`constructorFieldDialog`,HTMLDialogElement),constructorFieldDialogBody:T(`constructorFieldDialogBody`,HTMLElement),webuiDialog:T(`webuiDialog`,HTMLDialogElement),webuiDialogBody:T(`webuiDialogBody`,HTMLElement)};function A(){yt(),k.benchmarkModelSelect.innerHTML=bt().map(e=>`
    <option value="${d(xt(e))}" ${xt(e)===r.benchmark.modelKey?`selected`:``}>
      ${u(St(e))}
    </option>
  `).join(``),k.benchmarkTypeSelect.value=r.benchmark.type,k.benchmarkAllSections.checked=Ct(),k.benchmarkSections.innerHTML=l.map(e=>`
    <label class="toggle-row">
      <input type="checkbox" value="${d(e)}" data-benchmark-section="${d(e)}" ${r.benchmark.sections.includes(e)?`checked`:``} ${r.benchmark.type===`general`||Ct()?`disabled`:``}>
      <span>${u(e)}</span>
    </label>
  `).join(``),k.runBenchmarkButton.disabled=r.benchmark.running||!j(),ft(),pt()}async function ot(){let e=j();if(!e){r.benchmark.record=null,A();return}r.benchmark.error=``,r.benchmark.record=await de(e.node_id||``,e.local_id),A()}async function st(){let e=j();if(e){r.benchmark.running=!0,r.benchmark.error=``,A();try{r.benchmark.record=await fe({node_id:e.node_id||``,model_id:e.local_id,type:r.benchmark.type,sections:r.benchmark.type===`general`||Ct()?[`all`]:r.benchmark.sections,iterations:1,timeout_seconds:1800})}catch(e){r.benchmark.error=e instanceof Error?e.message:String(e)}finally{r.benchmark.running=!1,A()}}}function ct(e){r.benchmark.modelKey=e,r.benchmark.record=null,A()}function lt(e){r.benchmark.type=e===`section`?`section`:`general`,A()}function ut(e){r.benchmark.sections=e?[...l]:[],A()}function dt(){let e=Array.from(k.benchmarkSections.querySelectorAll(`[data-benchmark-section]`)).filter(e=>e instanceof HTMLInputElement&&e.checked).map(e=>e.value).filter(wt);r.benchmark.sections=e,A()}function ft(){let e=vt(),t=e?.latest;if(r.benchmark.error){k.benchmarkLatest.innerHTML=`<div class="error-text">${u(r.benchmark.error)}</div>`;return}if(!t){k.benchmarkLatest.innerHTML=`<div class="detail-empty">No benchmark data</div>`;return}let n=l.map(t=>e?.sections?.[t]).filter(e=>!!e);k.benchmarkLatest.innerHTML=[mt(`Latest`,t),...n.map(e=>mt(e.section,e))].join(``)}function pt(){let e=vt()?.history??[];if(e.length===0){k.benchmarkHistory.innerHTML=`<div class="detail-empty">No history yet</div>`;return}k.benchmarkHistory.innerHTML=e.slice().reverse().map(e=>`
    <article class="benchmark-row">
      <div>
        <strong>${u(e.section)} / ${u(e.status)}</strong>
        <div class="muted">${Tt(e.finished_at)} / ${e.duration_ms||0}ms</div>
      </div>
      <div class="change-list">${_t(e)}</div>
    </article>
  `).join(``)}function mt(e,t){return`
    <article class="benchmark-card">
      <strong>${u(e)}</strong>
      <div class="benchmark-status ${d(t.status)}">${u(t.status)}</div>
      <div class="muted">${t.duration_ms||0}ms / ${Tt(t.finished_at)}</div>
      ${t.error?`<div class="error-text">${u(t.error)}</div>`:``}
      <div class="metric-list">${(t.metrics??[]).map(e=>`
        <span>${u(e.name)}: ${u(ht(e))}</span>
      `).join(``)}</div>
    </article>
  `}function ht(e){return e.duration_ms?`${e.duration_ms}ms`:e.value!==void 0&&e.unit?`${gt(e.value)} ${e.unit}`:e.value===void 0?e.status:gt(e.value)}function gt(e){return Number.isInteger(e)?e.toString():e.toFixed(2)}function _t(e){let t=e.option_changes??[];return t.length===0?`<span class="muted">no option changes</span>`:t.map(e=>`
    <span class="chip amber">${u(e.key)} ${u(e.kind)}</span>
  `).join(``)}function vt(){if(r.benchmark.record)return r.benchmark.record;let e=j();if(!e?.benchmark)return null;let t={node_id:e.node_id,model_id:e.local_id,history:[]};return e.benchmark.latest&&(t.latest=e.benchmark.latest),e.benchmark.sections&&(t.sections=e.benchmark.sections),t}function yt(){r.benchmark.modelKey&&j()||(r.benchmark.modelKey=xt(bt()[0]))}function j(){return bt().find(e=>xt(e)===r.benchmark.modelKey)??null}function bt(){return He()}function xt(e){return e?`${e.node_id}\n${e.local_id}`:``}function St(e){return`${e.node_id||`node`} / ${e.local_id||e.public_id}`}function Ct(){return r.benchmark.sections.length===l.length}function wt(e){return l.includes(e)}function Tt(e){return e?new Date(e).toLocaleString():`never`}var Et=[{value:`24h`,label:`Last 24 hours`},{value:`7d`,label:`Last 7 days`},{value:`30d`,label:`Last 30 days`},{value:`90d`,label:`Last 90 days`},{value:`all`,label:`All time`}],Dt=[{value:``,label:`All sections`},{value:`llm`,label:`LLM`},{value:`embed`,label:`Embeddings`},{value:`image`,label:`Images`},{value:`voice`,label:`Voice`},{value:`music`,label:`Music`}];function Ot(e){return[{value:``,label:`All nodes`},...Ft((e?.nodes??[]).map(e=>e.node_id).filter(It)).map(e=>({value:e,label:e}))]}function kt(e){return[{value:``,label:`All models`},...Ft((e?.nodes??[]).flatMap(e=>e.models??[]).map(e=>e.local_id||e.public_id).filter(It)).map(e=>({value:e,label:e}))]}function At(e){let t={period:e.period||`24h`};return e.node_id&&(t.node_id=e.node_id),e.model_id&&(t.model_id=e.model_id),e.section&&(t.section=e.section),t}function M(e){return Math.round(Number.isFinite(e??0)?e??0:0).toLocaleString(`en-US`)}function N(e,t=1){let n=Number.isFinite(e??0)?e??0:0;return Number.isInteger(n)?n.toLocaleString(`en-US`):n.toLocaleString(`en-US`,{maximumFractionDigits:t,minimumFractionDigits:n>0&&n<10?t:0})}function P(e){let t=Number.isFinite(e??0)?e??0:0;if(t<60)return`${N(t,1)}s`;let n=t/60;return n<60?`${N(n,1)}m`:`${N(n/60,1)}h`}function F(e){return`${M(e)} MB`}function jt(e){return`${N(e,1)}%`}function Mt(e,t,n){let r={points:[],linePath:``,ticks:[]};if(e.length===0||t<=0||n<=0)return r;let i=Math.max(...e.map(e=>e.request_count),1),a=Math.max(0,t-8),o=Math.max(0,n-8),s=e.length-1,c=e.map((e,t)=>({x:4+(s===0?.5:t/s)*a,y:4+(1-e.request_count/i)*o,radius:4}));return{points:c,linePath:c.map((e,t)=>`${t===0?`M`:`L`} ${e.x.toFixed(2)} ${e.y.toFixed(2)}`).join(` `),ticks:Nt(e,c)}}function Nt(e,t){if(e.length===0||t.length===0)return[];let n=e.length-1,r=n<=3?e.map((e,t)=>t):[0,Math.round(n/3),Math.round(n*2/3),n];return Array.from(new Set(r)).map(n=>({x:t[n]?.x??0,label:Pt(e[n]?.bucket_start)}))}function Pt(e){return e?new Date(e).toLocaleDateString(`en-US`,{month:`short`,day:`numeric`}):``}function Ft(e){return Array.from(new Set(e)).sort((e,t)=>e.localeCompare(t))}function It(e){return!!e?.trim()}async function I(){r.analytics.loading=!0,r.analytics.error=``,Lt();try{r.analytics.data=await pe(At(r.analytics.query))}catch(e){r.analytics.error=e instanceof Error?e.message:String(e)}finally{r.analytics.loading=!1,Lt()}}function Lt(){Ht();let e=r.analytics.data;r.analytics.error?k.analyticsStatus.innerHTML=`<div class="error-text">${u(r.analytics.error)}</div>`:r.analytics.loading?k.analyticsStatus.innerHTML=`<div class="detail-empty">Loading analytics</div>`:e?.enabled?k.analyticsStatus.innerHTML=``:k.analyticsStatus.innerHTML=`<div class="detail-empty">Analytics disabled</div>`,Ut(),Wt(),Gt(),Kt()}function Rt(e){an(e)&&(r.analytics.query.period=e)}function zt(e){e?r.analytics.query.node_id=e:delete r.analytics.query.node_id}function Bt(e){e?r.analytics.query.model_id=e:delete r.analytics.query.model_id}function Vt(e){e?r.analytics.query.section=e:delete r.analytics.query.section}function Ht(){let e=At(r.analytics.query);k.analyticsPeriodSelect.innerHTML=nn(Et,e.period),k.analyticsNodeSelect.innerHTML=nn(rn(Ot(r.inventory),e.node_id),e.node_id||``),k.analyticsModelSelect.innerHTML=nn(rn(kt(r.inventory),e.model_id),e.model_id||``),k.analyticsSectionSelect.innerHTML=nn(Dt,e.section||``)}function Ut(){let e=r.analytics.data?.summary;if(!r.analytics.data?.enabled||!e){k.analyticsSummary.innerHTML=``;return}k.analyticsSummary.innerHTML=[L(`Requests`,M(e.request_count),`${M(e.success_count)} ok / ${M(e.failure_count)} failed`),L(`Tokens`,M(e.total_tokens),`${M(e.input_tokens)} in / ${M(e.output_tokens)} out`),L(`Speed`,`${N(e.average_tokens_per_second,1)} tok/s`,`${N(e.average_duration_ms,0)}ms avg`),L(`Images`,M(e.image_count),`generated or returned`),L(`Audio`,P(e.audio_seconds),`${M(e.audio_tokens)} tokens`),L(`VRAM`,F(e.vram_peak_mb),`${jt(e.vram_peak_percent)} peak / ${F(e.vram_total_mb)} total`),L(`Loads`,M(e.load_count),`${N(e.average_load_duration_ms,0)}ms avg / ${F(e.model_vram_estimate_mb)} model`)].join(``)}function Wt(){let e=r.analytics.data?.timeline??[];if(!r.analytics.data?.enabled||e.length===0){k.analyticsTimeline.innerHTML=``;return}let t=Mt(e,720,170);k.analyticsTimeline.innerHTML=`
    <div class="analytics-chart-head">
      <strong>Timeline</strong>
      <span class="muted">${u(r.analytics.data.granularity)}</span>
    </div>
    <svg class="analytics-chart" viewBox="0 0 720 220" role="img" aria-label="Analytics timeline">
      <path class="analytics-line" d="${d(t.linePath)}"></path>
      ${t.points.map((t,n)=>{let r=e[n];return r?`
        <circle class="analytics-point" cx="${t.x.toFixed(2)}" cy="${t.y.toFixed(2)}" r="${t.radius.toFixed(2)}">
          <title>${u(sn(r))}: ${M(r.request_count)} requests</title>
        </circle>
      `:``}).join(``)}
      <line class="analytics-axis" x1="4" y1="180" x2="716" y2="180"></line>
      ${t.ticks.map(e=>`
        <g class="analytics-tick">
          <line class="analytics-axis" x1="${e.x.toFixed(2)}" y1="175" x2="${e.x.toFixed(2)}" y2="185"></line>
          <text class="analytics-tick-label" x="${e.x.toFixed(2)}" y="204">${u(e.label)}</text>
        </g>
      `).join(``)}
    </svg>
  `}function Gt(){let e=r.analytics.data?.sections??[];if(!r.analytics.data?.enabled||e.length===0){k.analyticsSections.innerHTML=``;return}let t=Math.max(...e.map(e=>e.request_count),1);k.analyticsSections.innerHTML=`
    <div class="analytics-chart-head">
      <strong>Sections</strong>
      <span class="muted">requests by lane</span>
    </div>
    <div class="analytics-section-bars">
      ${e.map(e=>qt(e,t)).join(``)}
    </div>
  `}function Kt(){let e=r.analytics.data;if(!e?.enabled){k.analyticsModelsTable.innerHTML=``,k.analyticsNodesTable.innerHTML=``,k.analyticsRecentTable.innerHTML=``,k.analyticsNodeErrors.innerHTML=``;return}k.analyticsModelsTable.innerHTML=e.models.map(Jt).join(``),k.analyticsNodesTable.innerHTML=e.nodes.map(Yt).join(``),k.analyticsRecentTable.innerHTML=e.recent.map(Xt).join(``),k.analyticsNodeErrors.innerHTML=(e.node_errors??[]).map(e=>`
    <div class="error-text">${u(e.node_id||e.node_url||`node`)}: ${u(e.error)}</div>
  `).join(``)}function L(e,t,n){return`
    <article class="analytics-metric">
      <span>${u(e)}</span>
      <strong>${u(t)}</strong>
      <small>${u(n)}</small>
    </article>
  `}function qt(e,t){let n=Math.max(1,Math.round(e.request_count/t*100));return`
    <div class="analytics-section-row">
      <span>${u(on(e.section))}</span>
      <svg viewBox="0 0 100 8" role="img" aria-label="${d(e.section)} requests">
        <rect class="analytics-bar-track" x="0" y="0" width="100" height="8"></rect>
        <rect class="analytics-bar" x="0" y="0" width="${n}" height="8"></rect>
      </svg>
      <strong>${M(e.request_count)}</strong>
    </div>
  `}function Jt(e){return`
    <tr>
      <td>${u(e.node_id)}</td>
      <td>${u(e.model_id||`unknown`)}</td>
      <td>${M(e.request_count)}</td>
      <td>${M(e.load_count)}</td>
      <td>${F(e.vram_peak_mb)} / ${jt(e.vram_peak_percent)}</td>
      <td>${M(e.total_tokens)}</td>
      <td>${M(e.image_count)}</td>
      <td>${P(e.audio_seconds)}</td>
    </tr>
  `}function Yt(e){return`
    <tr>
      <td>${u(e.node_id)}</td>
      <td>${M(e.request_count)}</td>
      <td>${M(e.load_count)}</td>
      <td>${F(e.vram_peak_mb)} / ${jt(e.vram_peak_percent)}</td>
      <td>${M(e.total_tokens)}</td>
      <td>${M(e.image_count)}</td>
      <td>${P(e.audio_seconds)}</td>
    </tr>
  `}function Xt(e){let t=e.event_type===`model_load`?en(e):e.section===`image`?Qt(e):e.section===`voice`||e.section===`music`?$t(e):Zt(e);return`
    <tr>
      <td>${u(cn(e.finished_at))}</td>
      <td>${u(e.node_id)}</td>
      <td>${u(e.model_id||`unknown`)}</td>
      <td>${u(on(e.section))}</td>
      <td>${u(e.backend_mode||``)}</td>
      <td>${u(e.success?`ok`:String(e.status_code))}</td>
      <td>${u(t)}</td>
    </tr>
  `}function Zt(e){let t=e.tokens_per_second?` / ${N(e.tokens_per_second,1)} tok/s`:``;return`${M(e.input_tokens)} in / ${M(e.output_tokens)} out${t}${tn(e)}`}function Qt(e){let t=e.image_width&&e.image_height?` / ${e.image_width}x${e.image_height}`:``;return`${e.image_type?`${e.image_type} / `:``}${M(e.image_count)} images${t}${tn(e)}`}function $t(e){return`${P(e.audio_seconds)} / ${M(e.audio_tokens)} tokens${tn(e)}`}function en(e){return`${e.config_filename?`${e.config_filename} / `:``}${N(e.duration_ms,0)}ms / ${F(e.load_vram_before_mb)} -> ${F(e.load_vram_after_mb)} / +${F(e.load_vram_delta_mb)}`}function tn(e){return!e.work_vram_max_mb&&!e.model_vram_estimate_mb?``:` / VRAM ${F(e.work_vram_max_mb)} (${jt(e.vram_peak_percent)}) / model ${F(e.model_vram_estimate_mb)}`}function nn(e,t){return e.map(e=>`
    <option value="${d(e.value)}" ${e.value===t?`selected`:``}>${u(e.label)}</option>
  `).join(``)}function rn(e,t){return!t||e.some(e=>e.value===t)?e:[...e,{value:t,label:t}]}function an(e){return e===`24h`||e===`7d`||e===`30d`||e===`90d`||e===`all`}function on(e){return Dt.find(t=>t.value===e)?.label??e}function sn(e){return cn(e.bucket_start)}function cn(e){return e?new Date(e).toLocaleString():`never`}var R=[`text`,`image`,`embeddings`,`voice`,`music`],z=`backend_mode`,ln=`router_unload_policy`,B=[`kobold`,`llama_sdcpp`],un=[`none`,...R,`all`];[...R];var dn={kobold:`Kobold`,llama_sdcpp:`llama/sd.cpp`},fn={none:`None`,text:`Text`,image:`Image`,embeddings:`Embeddings`,voice:`Voice`,music:`Music`,all:`All`},V={text:{label:`LLM`,shortLabel:`Text`,section:`llm`,accent:`cyan`,dropLabel:`Drop a text config or model file`},image:{label:`Image`,shortLabel:`Image`,section:`image`,accent:`magenta`,dropLabel:`Drop an image config or model file`},embeddings:{label:`Embed`,shortLabel:`Embed`,section:`embed`,accent:`lime`,dropLabel:`Drop an embedding config or model file`},voice:{label:`Voice`,shortLabel:`Voice`,section:`voice`,accent:`amber`,dropLabel:`Drop Whisper, TTS, tokenizer, or voice dir`},music:{label:`Music`,shortLabel:`Music`,section:`music`,accent:`violet`,dropLabel:`Drop Music LLM, embeddings, diffusion, or VAE`}};function H(e){return R.includes(e)}var pn={voice:[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],music:[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]};function mn(e){return e===`voice`||e===`music`?pn[e]:[]}function hn(e,t){return(t===`voice`||t===`music`)&&e.component.source===`file`&&!e.component.option_key}function gn(e,t){return e!==`voice`&&e!==`music`?!1:pn[e].includes(t)}function _n(e,t){let n={};for(let[r,i]of Object.entries(e))bn(i)!==bn(t[r])&&(n[r]=i);return n}function vn(e){return JSON.parse(JSON.stringify(e||{}))}function yn(e){return`${e.backendMode}\n${e.section}\n${e.name}`}function bn(e){return JSON.stringify(e??null)??``}var xn=[`llm`,`image`,`embed`,`voice`,`music`,`runtime`,`other`],Sn={llm:`LLM`,image:`Image`,embed:`Embed`,voice:`Voice`,music:`Music`,runtime:`Runtime`,other:`Other`},Cn={llm:[`model_param`,`model`],image:[`sdmodel`],embed:[`embeddingsmodel`,`mmproj`],voice:[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],music:[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]};function U(){return x(r.simpleCook.nodeID)??(r.inventory?.nodes??[])[0]??null}function W(){return(U()?.models??[]).find(e=>e.local_id===r.simpleCook.configID)??null}function wn(){let e=U(),t=e?.node_id||``,n=e?.models??[];return{node:e,nodeFiles:Ue().filter(e=>e.node_id===t),nodeModels:n,otherNodeModels:n.filter(e=>e.local_id!==r.simpleCook.configID),comparableBySection:new Map}}function Tn(e,t){let n=new Map(xn.map(e=>[e,[]]));for(let r of Object.keys(e).sort((e,t)=>e.localeCompare(t))){let e=Rn(t(r)),i=n.get(e)??[];i.push(r),n.set(e,i)}return xn.map(e=>({section:e,keys:n.get(e)??[]})).filter(e=>e.keys.length>0)}function En(e,t,n){return Un([...t?.choices??[],...Fn(t,n),...In(e,n)].map(e=>Hn(e,t)))}function Dn(e,t,n){let i=r.simpleCook.fields[e],a=zn(r.simpleCook.fields,t),o=Ln(t,n),s=o.map(t=>t.options?.[e]).filter(e=>!v(e));if(s.length===0)return a&&o.length===0&&!v(i)?`compare-same`:`compare-none`;let c=ze(i);return s.every(e=>ze(e)===c)?`compare-same`:`compare-different`}function On(e,t,n,r){let i=Rn(n(e)),a=t===`model`?Ln(i,r):r.otherNodeModels,o=[],s=new Set;for(let t of a){let n=t.options?.[e];if(v(n))continue;let r=_(n),i=`${r}\n${t.local_id}`;s.has(i)||(s.add(i),o.push({value:r,config:Mn(t)}))}return o}function kn(e){let t=e?.hardware,n={quiet:!0,nomodel:!1,contextsize:4096,threads:t?.max_threads?Math.max(1,Math.floor(t.max_threads/2)):-1,batchsize:512,usemmap:!0,usemlock:!1,gpulayers:t?.gpu_backend&&t.gpu_backend!==`cpu`&&t.gpu_backend!==`unknown`?`auto`:`0`};(t?.gpu_backend===`cuda`||t?.gpu_backend===`rocm`)&&(n.usecuda=!0),t?.gpu_backend===`vulkan`&&(n.usevulkan=!0);let r=Wn(e?.node_url||``);return r&&(n.host=r.hostname,r.port&&(n.port=Number(r.port))),n}function An(e){if(e?.default!==void 0&&e.default!==``)return Re(e,e.default);switch(e?.value_type){case`bool`:return!1;case`number`:return 0;case`json`:return{};default:return``}}function G(e){return JSON.parse(JSON.stringify(e||{}))}function jn(e){return`${e.node_id||`node`} / ${e.backend_mode||`backend`}`}function Mn(e){return`${e.local_id||e.public_id||`config`} / ${e.filename||``}`}function Nn(e,t){return`${(e?.node_id||`node`).toLowerCase().replace(/[^a-z0-9_-]+/g,`-`).replace(/^-|-$/g,``)||`node`}-${t}`}function Pn(e){return String(e).replace(/[^a-z0-9_-]/gi,`-`)}function Fn(e,t){if(!e?.model_role)return[];let n=t.nodeFiles.filter(t=>Bn(h(t),e.model_role??``)).map(e=>e.path),r=t.nodeModels.flatMap(t=>Vn(t,e.model_role??``));return[...n,...r]}function In(e,t){return t.nodeModels.map(t=>t.options?.[e]).filter(e=>!v(e)).map(_)}function Ln(e,t){let n=t.comparableBySection.get(e);if(n)return n;let i=zn(r.simpleCook.fields,e),a=t.otherNodeModels;return i?(a=a.filter(t=>zn(t.options??{},e)===i),t.comparableBySection.set(e,a),a):(t.comparableBySection.set(e,a),a)}function Rn(e){return e?.section||`other`}function zn(e,t){for(let n of Cn[t]??[]){let t=e?.[n];if(!v(t))return ze(t)}return``}function Bn(e,t){return t===`llm`?e.includes(`llm`):t===`image`?e.includes(`image`):t===`embeddings`?e.includes(`embeddings`)||e.includes(`llm`):t===`multimodal`?e.includes(`multimodal`):t===`vae`?e.includes(`vae`):t===`clip`?e.includes(`clip`):t===`t5`?e.includes(`t5`):t===`upscaler`?e.includes(`upscaler`):t===`lora`?e.includes(`lora`):t===`voice`?e.includes(`voice`):t===`music`?e.includes(`music`):!0}function Vn(e,t){let n=e.capabilities??{},r=[];return t===`llm`&&typeof e.filename==`string`&&r.push(e.filename),t===`image`&&n.image?.model&&r.push(n.image.model),t===`embeddings`&&n.embeddings?.model&&r.push(n.embeddings.model),t===`multimodal`&&n.multimodal?.projector&&r.push(n.multimodal.projector),t===`vae`&&n.image?.vae&&r.push(n.image.vae),t===`clip`&&r.push(n.image?.clip1,n.image?.clip2,n.image?.clip_l,n.image?.clip_g),t===`t5`&&n.image?.t5xxl&&r.push(n.image.t5xxl),t===`upscaler`&&n.image?.upscaler&&r.push(n.image.upscaler),t===`lora`&&r.push(...n.image?.lora??[]),t===`voice`&&r.push(n.voice?.whisper_model,n.voice?.tts_model,n.voice?.wav_tokenizer,n.voice?.directory),t===`music`&&r.push(n.music?.llm,n.music?.embeddings,n.music?.diffusion,n.music?.vae),r.filter(e=>!!e)}function Hn(e,t){if(t?.value_type===`json`)try{return JSON.parse(e),e}catch{return JSON.stringify(e)}return e}function Un(e){let t=new Set,n=[];for(let r of e){let e=String(r??``).trim();!e||t.has(e)||(t.add(e),n.push(e))}return n}function Wn(e){try{return new URL(e)}catch{return null}}var Gn=`tensors-router.constructorFieldPresets`;function Kn(){if(!(r.constructor.fieldPresets.length>0))try{let e=JSON.parse(window.localStorage.getItem(Gn)||`[]`);r.constructor.fieldPresets=Array.isArray(e)?e.filter(gr):[]}catch{r.constructor.fieldPresets=[]}}function qn(e,t){Kn(),r.constructor.fieldEditor={lane:e,draft:vn(r.constructor.laneOptions[e])},t&&(r.constructor.fieldEditor.pendingPayload=t),K(),hr()}function Jn(){r.constructor.fieldEditor=null,k.constructorFieldDialog.close(),k.constructorFieldDialogBody.innerHTML=``}function K(){let e=r.constructor.fieldEditor;if(!e){k.constructorFieldDialog.open&&Jn();return}let t=e.lane,n=V[t],i=lr(e.pendingPayload??r.constructor.lanes[t]),a=ar(t,i,e.draft);k.constructorFieldDialogBody.innerHTML=`
    <div class="field-dialog-head">
      <div>
        <h3>${u(n.label)} Fields</h3>
        <p class="muted">${u(n.section)} staged overrides</p>
      </div>
      <button class="icon-button" type="button" title="Close" data-field-modal-action="cancel">x</button>
    </div>
    ${e.pendingPayload?ir(t,e.pendingPayload):``}
    <div class="preset-row">
      <label>
        Preset
        <select data-field-preset-select>${sr(t)}</select>
      </label>
      <button type="button" data-field-modal-action="apply-preset">Apply Preset</button>
      <label>
        Save as
        <input data-field-preset-name type="text" placeholder="Preset name">
      </label>
      <button type="button" data-field-modal-action="save-preset">Save Preset</button>
    </div>
    <div class="field-add-row">
      <label>
        Add section field
        <select data-field-add-select>${or(t,a)}</select>
      </label>
      <button type="button" data-field-modal-action="add-field">Add Field</button>
    </div>
    <div class="field-diff-grid">
      ${a.map(t=>rr(t,i[t],e.draft)).join(``)||`<div class="detail-empty">No fields in this section</div>`}
    </div>
    <div class="field-dialog-actions">
      <button type="button" data-field-modal-action="reset-section">Reset Section</button>
      <span></span>
      <button type="button" data-field-modal-action="cancel">Cancel</button>
      <button type="button" data-field-modal-action="apply">Apply</button>
    </div>
  `}function Yn(e){let t=r.constructor.fieldEditor;if(!t||!(e instanceof HTMLInputElement))return;let n=e.dataset.fieldDraft;if(n)try{t.draft[n]=Re(b(n),e.value),e.setCustomValidity(``),K()}catch{e.setCustomValidity(`Invalid JSON`),e.reportValidity()}}function Xn(e,t){let n=e instanceof HTMLElement?e.closest(`[data-field-modal-action]`):null;if(!(n instanceof HTMLElement))return;let r=n.dataset.fieldModalAction;if(r===`cancel`){Jn();return}if(r===`apply`){Zn(),t();return}if(r===`reset-section`){Qn();return}if(r===`reset-field`){$n(n.dataset.fieldKey||``);return}if(r===`add-field`){er();return}if(r===`apply-preset`){tr();return}r===`save-preset`&&nr()}function Zn(){let e=r.constructor.fieldEditor;if(!e)return;if(e.pendingPayload){let t=dr();if(!gn(e.lane,t)){k.constructorFieldDialogBody.querySelector(`[data-file-option-key]`)?.setAttribute(`aria-invalid`,`true`);return}r.constructor.lanes[e.lane]=fr(e.pendingPayload,t)}let t=lr(r.constructor.lanes[e.lane]);r.constructor.laneOptions[e.lane]=_n(e.draft,t),Jn()}function Qn(){let e=r.constructor.fieldEditor;e&&(e.draft={},K())}function $n(e){let t=r.constructor.fieldEditor;t&&(delete t.draft[e],K())}function er(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-add-select]`);!e||!(t instanceof HTMLSelectElement)||!t.value||(e.draft[t.value]=An(b(t.value)),K())}function tr(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-preset-select]`);if(!e||!(t instanceof HTMLSelectElement)||!t.value)return;let n=cr(e.lane).find(e=>yn(e)===t.value);n&&(Object.assign(e.draft,vn(n.values)),K())}function nr(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-preset-name]`);if(!e||!(t instanceof HTMLInputElement)||!t.value.trim())return;let n={name:t.value.trim(),backendMode:ur(e),section:V[e.lane].section,values:vn(e.draft)};r.constructor.fieldPresets=[...r.constructor.fieldPresets.filter(e=>yn(e)!==yn(n)),n],window.localStorage.setItem(Gn,JSON.stringify(r.constructor.fieldPresets)),K()}function rr(e,t,n){let r=b(e),i=Object.hasOwn(n,e),a=i?n[e]:void 0,o=i&&bn(a)!==bn(t);return`
    <div class="field-diff-row ${o?`changed`:``}">
      <div class="field-label">
        <span>${u(r?.name||e)}</span>
        <code>${u(e)}</code>
      </div>
      <div class="field-source">
        <span class="muted">Source</span>
        <strong>${u(_(t)||`inherit`)}</strong>
      </div>
      <label class="field-override">
        Override
        <input data-field-draft="${d(e)}" value="${d(i?g(a):``)}" placeholder="inherit">
      </label>
      <div class="field-state">
        ${i?p(o?`changed`:`same`,o?`amber`:`violet`):p(`source`,``)}
        <button class="icon-button" type="button" title="Reset field" data-field-modal-action="reset-field" data-field-key="${d(e)}">x</button>
      </div>
    </div>
  `}function ir(e,t){if(!hn(t,e))return``;let n=mn(e);return`
    <div class="assignment-panel">
      <div>
        <strong>${u(t.label)}</strong>
        <p class="muted">${u(t.subtitle)}</p>
      </div>
      <label>
        Assign file to
        <select data-file-option-key>
          ${n.map(e=>`<option value="${d(e)}">${u(e)}</option>`).join(``)}
        </select>
      </label>
    </div>
  `}function ar(e,t,n){let r=V[e].section,i=new Set;for(let e of y())(e.section||`other`)===r&&i.add(e.key);for(let a of[...Object.keys(t),...Object.keys(n),...pr(e)]){let e=b(a);(!e||(e.section||`other`)===r)&&i.add(a)}return Array.from(i).sort((e,t)=>e.localeCompare(t))}function or(e,t){let n=new Set(t),r=V[e].section;return y().filter(e=>(e.section||`other`)===r&&!n.has(e.key)).sort(mr).map(e=>`<option value="${d(e.key)}">${u(e.key)}</option>`).join(``)}function sr(e){return cr(e).map(e=>`<option value="${d(yn(e))}">${u(e.name)}</option>`).join(``)}function cr(e){let t=r.constructor.fieldEditor,n=V[e].section,i=t?ur(t):``;return r.constructor.fieldPresets.filter(e=>e.section===n&&(!e.backendMode||e.backendMode===i))}function lr(e){return vn(e?.model?.options??{})}function ur(e){let t=e.pendingPayload??r.constructor.lanes[e.lane];return t?.model?.backend_mode?t.model.backend_mode:x(t?.component.node_id||``)?.backend_mode||`unknown`}function dr(){let e=k.constructorFieldDialogBody.querySelector(`[data-file-option-key]`);return e instanceof HTMLSelectElement?e.value:``}function fr(e,t){return{...e,component:{...e.component,option_key:t}}}function pr(e){return mn(e)}function mr(e,t){return e.key.localeCompare(t.key)}function hr(){k.constructorFieldDialog.open||k.constructorFieldDialog.showModal()}function gr(e){if(!e||typeof e!=`object`)return!1;let t=e;return typeof t.name==`string`&&typeof t.backendMode==`string`&&typeof t.section==`string`&&!!t.values&&typeof t.values==`object`&&!Array.isArray(t.values)}function _r(e,t){let n=vr(e),r={};for(let[e,i]of Object.entries(t)){let t=b(e);(!t||t.section===`runtime`||t.section===n)&&(r[e]=i)}return r}function vr(e){return V[e].section}function yr(){let e=Object.entries(r.constructor.lanes).filter(e=>e[1]!==null),t=e.map(([e,t])=>Sr(e,t)),n={};for(let[t,i]of e)Object.assign(n,_r(t,i.model?.options??{})),Object.assign(n,r.constructor.laneOptions[t]??{});return Object.assign(n,r.constructor.options),r.constructor.backendTouched&&(n[z]=r.constructor.backendMode),{id:k.advancedCookIdInput.value.trim(),overwrite:k.overwriteInput.checked,components:t,options:n}}function br(){let e=[],t=yr();t.id||e.push(m(`warning`,`id_missing`,`Config id is empty.`,`id`)),t.components.length===0&&e.push(m(`warning`,`empty_constructor`,`No lanes selected.`,``));for(let[n,r]of Ke(t.components)){let i=x(n),a=Je(n,r,t.options??{}),o=xr(a,i?.backend_mode||`kobold`);o===`kobold`&&Pe(r,`image`)&&Pe(r,`embeddings`)&&e.push(m(`error`,`kobold_image_embeddings_mix`,`Kobold cannot cook image and embeddings into the same config.`,n));let s=i?.hardware?.max_threads||0;for(let i of qe(n,r,t.options??{}))s>0&&i.value>s&&e.push(m(`error`,`thread_budget_exceeded`,`${i.key} uses ${i.value} threads on a node with ${s} logical CPUs.`,i.key));if(i?.hardware?.gpu_backend===`rocm`)for(let[t,n]of Object.entries(a))b(t)?.cuda_only&&Ie(n)&&e.push(m(`error`,`cuda_on_rocm`,`${t} is CUDA-only on a ROCm node.`,t));if(!i?.hardware?.gpu_backend||i.hardware.gpu_backend===`unknown`){for(let[t,r]of Object.entries(a))if(Fe(t)&&Ie(r)){e.push(m(`warning`,`gpu_backend_unknown`,`GPU backend could not be inferred.`,n));break}}for(let[t]of Object.entries(a)){let n=b(t);n?.known&&(n.backends?.length??0)>0&&!(n.backends??[]).includes(o)&&e.push(m(`warning`,`unsupported_option`,`${t} is not marked as supported by ${o}.`,t))}}return e}function xr(e,t){let n=e[z];return typeof n==`string`&&B.includes(n)?n:B.includes(t)?t:`kobold`}function Sr(e,t){let n=r.constructor.targetNodes[e]||t.component.node_id||``,i=x(n),a={...t.component,node_id:n,node_url:i?.node_url||t.component.node_url||``};if(n&&t.component.node_id&&n!==t.component.node_id){let n=Cr(e,t);n.path&&(a.source=`file`,a.file_path=n.path,n.optionKey?a.option_key=n.optionKey:delete a.option_key,delete a.model_id,delete a.image_id)}return a}function Cr(e,t){let n=t.model?.options??{};return e===`image`?{path:Tr(n.sdmodel)||t.file?.path||``}:e===`embeddings`?{path:Tr(n.embeddingsmodel)||t.file?.path||``}:e===`voice`?wr(n,[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],t.file?.path):e===`music`?wr(n,[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`],t.file?.path):{path:Tr(n.model_param)||Er(n.model)||t.file?.path||``}}function wr(e,t,n){for(let n of t){let t=Tr(e[n]);if(t)return{path:t,optionKey:n}}let r=n||``;if(!r)return{path:``};let i=t[0];return i?{path:r,optionKey:i}:{path:r}}function Tr(e){return typeof e==`string`?e.trim():``}function Er(e){if(typeof e==`string`)return e.trim();if(Array.isArray(e)){for(let t of e)if(typeof t==`string`&&t.trim())return t.trim()}return``}function q(){Lr(),Rr(),zr(),Br(),K()}function Dr(e,t){if(!e)return;if(e.type===`option`){Or(e.key);return}let n=H(t)?t:e.lane;if(n===e.lane){if(hn(e,n)){qn(n,e);return}r.constructor.lanes[n]=e,q()}}function Or(e){let t=b(e);t&&(Object.hasOwn(r.constructor.options,e)||(r.constructor.options[e]=Yr(t)),q())}function kr(){r.constructor.lanes=e(),r.constructor.targetNodes=t(),r.constructor.laneOptions=n(),r.constructor.backendMode=`kobold`,r.constructor.backendTouched=!1,r.constructor.options={},r.constructor.fieldEditor=null,q()}function Ar(e){H(e)&&(r.constructor.lanes[e]=null,r.constructor.laneOptions[e]={},q())}function jr(e){!H(e)||!r.constructor.lanes[e]||qn(e)}function Mr(e){if(!(e instanceof HTMLInputElement))return;let t=e.dataset.optionInput;if(t)try{r.constructor.options[t]=Re(b(t),e.value),e.setCustomValidity(``),Vr()}catch{e.setCustomValidity(`Invalid JSON`),e.reportValidity()}}function Nr(e){delete r.constructor.options[e],q()}function Pr(e){e===`used`&&(r.constructor.showUsedAll=!r.constructor.showUsedAll),e===`options`&&(r.constructor.showOptionsAll=!r.constructor.showOptionsAll),Br()}function Fr(e){if(!(e instanceof HTMLSelectElement))return;let t=e.dataset.laneTarget;H(t)&&(r.constructor.targetNodes[t]=e.value,q())}function Ir(e){B.includes(e)&&(r.constructor.backendMode=e,r.constructor.backendTouched=!0,q())}function Lr(){let e=Zr();k.advancedBackendSelect.innerHTML=B.map(t=>{let n=t===e?` selected`:``;return`<option value="${d(t)}"${n}>${u(dn[t])}</option>`}).join(``),k.advancedBackendSelect.classList.toggle(`virtual-backend-select`,!r.constructor.backendTouched)}function Rr(){let e=k.constructorFilterInput.value.trim().toLowerCase(),t=Hr().filter(t=>!e||JSON.stringify(t).toLowerCase().includes(e));r.palettePayloads={},k.paletteList.innerHTML=t.map(e=>{let t=`payload-${Object.keys(r.palettePayloads).length}`;r.palettePayloads[t]=e.payload;let n=e.payload.type===`option`?`<button type="button" data-add-option="${d(e.payload.key)}">Add</button>`:`<button type="button" data-select-payload="${d(t)}">Use</button>`;return`
      <article class="palette-item" draggable="true" data-drag-payload="${d(t)}">
        <div class="palette-title">
          <strong>${u(e.title)}</strong>
          ${p(e.badge,e.color)}
        </div>
        <div class="muted">${u(e.subtitle)}</div>
        <div class="palette-meta">${e.meta.map(e=>p(e,``)).join(``)}</div>
        ${n}
      </article>
    `}).join(``)||`<div class="detail-empty">No items</div>`}function zr(){k.constructorLanes.innerHTML=R.map(Gr).join(``);for(let e of R){let t=document.querySelector(`[data-drop-lane="${e}"]`);if(!(t instanceof HTMLElement))continue;let n=r.constructor.lanes[e];if(!n){t.innerHTML=`<div class="lane-empty">${u(V[e].dropLabel)}</div>`;continue}let i=Object.keys(r.constructor.laneOptions[e]??{}).length;t.innerHTML=`
      <article class="selected-card">
        <strong>${u(n.label)}</strong>
        <div class="muted">${u(n.subtitle)}</div>
        <div class="palette-meta">${n.meta.map(e=>p(e,``)).join(``)}</div>
        ${n.component.option_key?`<div class="muted">Assigned to ${u(n.component.option_key)}</div>`:``}
        <label>
          Target node
          <select data-lane-target="${d(e)}">${Xr(e,n)}</select>
        </label>
        <div class="lane-card-actions">
          <button type="button" data-edit-lane-fields="${d(e)}">Edit fields</button>
          ${i?p(`${i} overrides`,V[e].accent):``}
        </div>
      </article>
    `}}function Br(){Vr();let e=Ur();k.usedModelsList.innerHTML=Jr(e,r.constructor.showUsedAll,`used`).join(``)||`<div class="detail-empty">No models selected</div>`;let t=Wr();k.selectedOptionsList.innerHTML=Jr(t,r.constructor.showOptionsAll,`options`).join(``)||`<div class="detail-empty">No options selected</div>`}function Vr(){let e=br();k.validationList.innerHTML=e.length?e.map(Ae).join(``):`<div class="detail-empty">Clean</div>`}function Hr(){return r.activePalette===`files`?Qe():r.activePalette===`options`?$e():Ze()}function Ur(){let e=[];for(let t of R){let n=r.constructor.lanes[t];if(n){e.push(`
      <div class="used-row">
        ${p(V[t].shortLabel,je(t))}
        <span>${u(n.label)}</span>
      </div>
    `);for(let t of Xe(n))e.push(`<div class="muted">${u(t)}</div>`)}}return e}function Wr(){let e=[],t=Ye();for(let[n,i]of Object.entries(t).sort(([e],[t])=>e.localeCompare(t)))if(Object.hasOwn(r.constructor.options,n))e.push(qr(n,r.constructor.options[n]));else if(Kr(n)){let t=Kr(n);e.push(`
        <div class="option-row">
          ${p(n,``)}
          ${t?p(`${V[t].shortLabel} override`,V[t].accent):``}
          <span class="muted">${u(_(i))}</span>
        </div>
      `)}else e.push(`
        <div class="option-row">
          ${p(n,``)}
          <span class="muted">${u(_(i))}</span>
        </div>
      `);return e}function Gr(e){let t=V[e];return`
    <section class="lane ${d(t.accent)}" data-lane="${d(e)}">
      <div class="lane-head">
        <div>
          <h3>${u(t.label)}</h3>
          <span>${u(t.section)}</span>
        </div>
        <button type="button" data-clear-lane="${d(e)}">Clear</button>
      </div>
      <div class="lane-drop" data-drop-lane="${d(e)}"></div>
    </section>
  `}function Kr(e){return R.find(t=>Object.hasOwn(r.constructor.laneOptions[t]??{},e))??null}function qr(e,t){return`
    <div class="option-editor">
      <span>${u(e)}</span>
      <input data-option-input="${d(e)}" value="${d(g(t))}">
      <button type="button" data-remove-option="${d(e)}">Remove</button>
    </div>
  `}function Jr(e,t,n){return e.length<=9||t?e.length>9?[...e,`<button class="link-button" type="button" data-toggle-list="${n}">Show less</button>`]:e:[...e.slice(0,9),`<button class="link-button" type="button" data-toggle-list="${n}">Show all ${e.length}</button>`]}function Yr(e){switch(e.value_type){case`bool`:return!1;case`number`:return 0;case`json`:return{};default:return``}}function Xr(e,t){let n=r.inventory?.nodes??[],i=r.constructor.targetNodes[e]||t.component.node_id||n[0]?.node_id||``;return r.constructor.targetNodes[e]||(r.constructor.targetNodes[e]=i),n.map(e=>{let t=e.node_id===i?` selected`:``;return`<option value="${d(e.node_id)}"${t}>${u(e.node_id||`node`)}</option>`}).join(``)}function Zr(){if(r.constructor.backendTouched&&B.includes(r.constructor.backendMode))return r.constructor.backendMode;for(let e of R){let t=r.constructor.lanes[e]?.model?.options?.[z];if(typeof t==`string`&&B.includes(t))return t}for(let e of R){let t=r.constructor.lanes[e];if(!t)continue;let n=x(r.constructor.targetNodes[e]||t.component.node_id||``)?.backend_mode||``;if(B.includes(n))return n}let e=r.inventory?.nodes?.[0]?.backend_mode||`kobold`;return B.includes(e)?e:`kobold`}async function Qr(){await ei(he,yr())}async function $r(e){let t=br().filter(e=>e.severity===`error`);if(t.length>0){q(),k.cookOutput.textContent=JSON.stringify({validation:t},null,2);return}await ei(ge,yr()),await e()}async function ei(e,t){try{let n=await e(t);k.cookOutput.textContent=JSON.stringify(n,null,2),q()}catch(e){k.cookOutput.textContent=JSON.stringify(xe(e),null,2),q()}}async function ti(e,t){let n=e.trim();if(n){ni(`Loading ${n}...`,!1);try{await me({model:n}),ni(`Loaded ${n}`,!1),await t()}catch(e){ni(e instanceof Error?e.message:String(e),!0)}}}function ni(e,t){k.modelsActionStatus.textContent=e,k.modelsActionStatus.classList.toggle(`error-text`,t)}function ri(e,t){let n=t.trim().toLowerCase();return n?e.filter(e=>[e.name,e.backend,e.backend_mode,e.lane,e.node_id,e.url,...e.compatible_models.map(e=>e.id)].join(` `).toLowerCase().includes(n)):e}function ii(e){let t=new Map;for(let n of e){let e=n.node_id||`local`;t.set(e,[...t.get(e)??[],n])}return Array.from(t.entries()).sort(([e],[t])=>e.localeCompare(t)).map(([e,t])=>({nodeID:e,entries:[...t].sort((e,t)=>e.name.localeCompare(t.name))}))}function ai(e){return e.enabled?e.requires_loaded_model&&!e.can_open_without_model&&!e.active?{openable:!1,reason:`needs_model`}:{openable:!0,reason:``}:{openable:!1,reason:`disabled`}}function oi(e){let t=ai(e);return{title:e.name,message:ci(t.reason),canEnable:!e.enabled,canLoad:e.compatible_models.length>0,models:si(e)}}function si(e){return[...e.compatible_models].sort((e,t)=>e.active===t.active?e.id.localeCompare(t.id):e.active?-1:1)}function ci(e){switch(e){case`disabled`:return`Enable this WebUI before opening.`;case`needs_model`:return`Load a compatible model before opening.`;default:return`Ready to open.`}}async function J(){r.webuis.loading=!0,r.webuis.error=``,Y();try{r.webuis.data=await ce()}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{r.webuis.loading=!1,Y()}}function li(e){r.webuis.filter=e,Y()}async function ui(e,t){r.webuis.action=t?`Enabled`:`Disabled`,r.webuis.error=``,Y();try{r.webuis.data=await le({id:e,enabled:t})}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{Y()}}function di(e){let t=xi(e);if(t){if(!ai(t).openable){_i(t);return}Si(t.url)}}function fi(e){let t=xi(e);t&&_i(t)}function pi(){k.webuiDialog.close()}async function mi(e,t,n){let i=xi(e);if(i){r.webuis.action=`Loading ${t||n||i.name}...`,r.webuis.error=``,Y();try{let a=await ue({id:e,model_id:t,image_id:n});if(await J(),xi(e)?.enabled&&a.url){pi(),Si(a.url);return}r.webuis.action=`Loaded ${a.model_id||a.image_id||i.name}`}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{Y()}}}function Y(){let e=ri(r.webuis.data?.data??[],r.webuis.filter);k.webuiStatus.textContent=yi(e.length),k.webuiStatus.classList.toggle(`error-text`,r.webuis.error!==``),k.webuiGrid.innerHTML=e.length?ii(e).map(hi).join(``):`<div class="detail-empty">No WebUIs</div>`}function hi(e){return`
    <section class="webui-node-group">
      <div class="webui-node-head">
        <h3>${u(e.nodeID)}</h3>
        <span class="pill">${e.entries.length} WebUIs</span>
      </div>
      <div class="webui-cards">
        ${e.entries.map(gi).join(``)}
      </div>
    </section>
  `}function gi(e){let t=ai(e);return`
    <article class="webui-card">
      <div class="webui-card-head">
        <div>
          <strong>${u(e.name)}</strong>
          <div class="webui-url">${u(e.url)}</div>
        </div>
        <label class="toggle-row">
          <input type="checkbox" data-webui-toggle="${d(e.id)}" ${e.enabled?`checked`:``}>
          <span>Enable</span>
        </label>
      </div>
      <div class="node-meta">
        ${p(e.backend,`cyan`)}
        ${p(e.backend_mode,`violet`)}
        ${p(e.lane,je(e.lane))}
        ${p(e.active?`active`:`idle`,e.active?`lime`:`amber`)}
      </div>
      <div class="webui-model-summary">${u(bi(e))}</div>
      <div class="webui-actions">
        <button type="button" data-webui-open="${d(e.id)}">Open</button>
        <button type="button" data-webui-details="${d(e.id)}">${t.openable?`Models`:`Resolve`}</button>
      </div>
    </article>
  `}function _i(e){let t=oi(e);k.webuiDialogBody.innerHTML=`
    <div class="field-dialog-head">
      <div>
        <h2>${u(t.title)}</h2>
        <p class="muted">${u(t.message)}</p>
      </div>
      <button type="button" data-webui-dialog-close>Close</button>
    </div>
    <div class="webui-url">${u(e.url)}</div>
    <div class="webui-dialog-actions">
      ${t.canEnable?`<button type="button" data-webui-enable="${d(e.id)}">Enable</button>`:``}
    </div>
    <div class="webui-model-list">
      ${t.canLoad?t.models.map(t=>vi(e,t)).join(``):`<div class="detail-empty">No compatible models</div>`}
    </div>
  `,k.webuiDialog.showModal()}function vi(e,t){return`
    <div class="webui-model-row">
      <div>
        <strong>${u(t.id)}</strong>
        <div class="muted">${u(t.filename)}</div>
      </div>
      <div class="node-meta">
        ${p(t.node_id,`cyan`)}
        ${p(t.active?`active`:`available`,t.active?`lime`:`amber`)}
      </div>
      <button type="button" data-webui-load="${d(e.id)}" data-webui-load-model="${d(t.model_id)}" data-webui-load-image="${d(t.image_id||``)}">Load</button>
    </div>
  `}function yi(e){return r.webuis.error?r.webuis.error:r.webuis.loading?`Loading...`:r.webuis.action||`${e} WebUIs`}function bi(e){let t=e.active_image_id||e.active_model_id;return t?`Active: ${t}`:`${e.compatible_models.length} compatible`}function xi(e){return(r.webuis.data?.data??[]).find(t=>t.id===e)}function Si(e){let t=window.open(e,`_blank`,`noopener,noreferrer`);t&&(t.opener=null)}var Ci=[z,ln];function X(){Li(),Ri(),zi(),Bi(),Vi()}function wi(e){r.simpleCook.nodeID=e,Qi((U()?.models??[])[0]??null),X()}function Ti(e){Qi((U()?.models??[]).find(t=>t.local_id===e)??null),X()}function Ei(e){r.simpleCook.fieldFilter=e,Bi()}function Di(e){if(!(e instanceof HTMLDetailsElement))return;let t=e.dataset.simpleSection;if(!t)return;let n=new Set(r.simpleCook.openSections);e.open?n.add(t):n.delete(t),r.simpleCook.openSections=Array.from(n)}function Oi(){let e=U();r.simpleCook.configID=``,r.simpleCook.mode=`new`,r.simpleCook.fields=kn(e),r.simpleCook.cleanFields={},r.simpleCook.openSections=[],k.cookIdInput.value=Nn(e,`new-config`),X()}function ki(){let e=W();r.simpleCook.mode=`copy`,r.simpleCook.configID=``,r.simpleCook.fields=G(r.simpleCook.fields),r.simpleCook.cleanFields={},r.simpleCook.openSections=[],k.cookIdInput.value=Nn(U(),`${e?.local_id||`config`}-copy`),X()}function Ai(){let e=k.simpleAddFieldSelect.value;if(!e||Object.hasOwn(r.simpleCook.fields,e))return;let t=b(e);r.simpleCook.fields[e]=An(t),X()}function ji(e){if(!(e instanceof HTMLInputElement)&&!(e instanceof HTMLSelectElement))return;if(e instanceof HTMLSelectElement&&e.dataset.simpleBackendMode!==void 0){r.simpleCook.fields[z]=e.value,X();return}let t=e.dataset.simpleField;t&&(r.simpleCook.fields[t]=Re(b(t),e.value),X())}function Mi(e){delete r.simpleCook.fields[e],r.simpleCook.sidebar?.key===e&&(r.simpleCook.sidebar=null),X()}function Ni(e,t){r.simpleCook.sidebar={key:e,type:t},Vi()}async function Pi(){await qi(ve)}async function Fi(e){let t=await qi(ye);t&&(r.simpleCook.mode=`edit`,r.simpleCook.configID=t.id||``,r.simpleCook.fields=G(t.options??r.simpleCook.fields),r.simpleCook.cleanFields=G(r.simpleCook.fields),await e())}async function Ii(e){let t=W();if(t&&window.confirm(`Delete ${t.filename||t.local_id}?`))try{await be({node_id:r.simpleCook.nodeID,node_url:U()?.node_url||``,id:t.local_id,filename:t.filename,overwrite:!1,options:{}}),await e()}catch(e){ea(e)}}function Li(){let e=r.inventory?.nodes??[];if(e.length===0){r.simpleCook.nodeID=``,r.simpleCook.configID=``,r.simpleCook.fields={},r.simpleCook.cleanFields={},r.simpleCook.openSections=[];return}if(!e.some(e=>e.node_id===r.simpleCook.nodeID)){let t=e[0];r.simpleCook.nodeID=t?.node_id??``,Qi((t?.models??[])[0]??null);return}if(r.simpleCook.mode!==`edit`)return;let t=W(),n=U();!t&&(n?.models??[]).length>0&&Qi((n?.models??[])[0]??null)}function Ri(){let e=r.inventory?.nodes??[];$i(k.simpleNodeSelect,e.map(e=>at(e.node_id,jn(e)))),k.simpleNodeSelect.value=r.simpleCook.nodeID;let t=U()?.models??[];$i(k.simpleConfigSelect,t.map(e=>at(e.local_id,Mn(e)))),k.simpleConfigSelect.value=r.simpleCook.configID,k.simpleConfigSelect.disabled=t.length===0,k.simpleCopyButton.disabled=Object.keys(r.simpleCook.fields||{}).length===0,k.simpleDeleteButton.disabled=!W(),k.simpleFieldFilter.value=r.simpleCook.fieldFilter}function zi(){let e=r.simpleCook.fields||{},t=y().filter(t=>!Ci.includes(t.key)&&!Object.hasOwn(e,t.key)).sort((e,t)=>`${ta(e)}:${e.key}`.localeCompare(`${ta(t)}:${t.key}`));k.simpleAddFieldSelect.innerHTML=t.map(e=>{let t=`${Sn[ta(e)]||`Other`} / ${e.key}`;return`<option value="${d(e.key)}">${u(t)}</option>`}).join(``)}function Bi(){let e=r.simpleCook.fields||{},t=r.simpleCook.fieldFilter.trim().toLowerCase(),n=wn(),i=new Set(r.simpleCook.openSections),a=Yi(e).map(r=>{let a=r.keys.filter(e=>!t||`${e} ${_(Xi(e))}`.toLowerCase().includes(t)),o=a.map(t=>Hi(t,Xi(t),r.section,n,t===`backend_mode`&&!Object.hasOwn(e,`backend_mode`))).join(``);if(!o)return null;let s=Sn[r.section]||r.section;return{section:r.section,html:`
        <details class="config-section" data-simple-section="${d(r.section)}"${i.has(r.section)?` open`:``}>
          <summary>
            <span>${u(s)}</span>
            <span class="section-count">${u(na(a.length))}</span>
          </summary>
          <div class="config-fields">${o}</div>
        </details>
      `}}).filter(e=>e!==null);k.simpleConfigEditor.innerHTML=a.length?a.map(e=>e.html).join(``):`<div class="detail-empty">No fields</div>`}function Vi(){let e=r.simpleCook.sidebar;if(!e){k.simpleFieldSidebar.innerHTML=`<div class="detail-empty">Field values</div>`;return}let t=On(e.key,e.type,b,wn());k.simpleFieldSidebar.innerHTML=`
    <div class="field-sidebar-head">
      <div>
        <h3>${u(e.key)}</h3>
        <p class="muted">${u(e.type===`model`?`same model file`:`same field`)}</p>
      </div>
      <button type="button" data-close-field-sidebar>x</button>
    </div>
    <div class="detail-list">
      ${t.length?t.map(Ki).join(``):`<div class="detail-empty">No values</div>`}
    </div>
  `}function Hi(e,t,n,r,i=!1){let a=b(e),o=`field-values-${Pn(e)}`,s=En(e,a,r),c=Dn(e,n,r),ee=Ui(e,t,o,s,i),te=Cn[n]?`<button class="icon-button" type="button" title="Same model values" data-field-model-values="${d(e)}">M</button>`:``;return`
    <div class="config-field ${c}${i?` backend-virtual`:``}">
      <div class="field-label">
        <span>${u(a?.name||e)}</span>
        <code>${u(e)}</code>
      </div>
      <div class="field-control">
        ${ee}
      </div>
      <div class="field-buttons">
        <button class="icon-button" type="button" title="Other config values" data-field-values="${d(e)}">V</button>
        ${te}
        ${i?``:`<button class="icon-button" type="button" title="Remove field" data-remove-simple-field="${d(e)}">x</button>`}
      </div>
    </div>
  `}function Ui(e,t,n,r,i){return e===`backend_mode`?Wi(Zi(),i):e===`router_unload_policy`?Gi(g(t),i):`
    <input data-simple-field="${d(e)}" list="${d(n)}" value="${d(g(t))}">
    <datalist id="${d(n)}">
      ${r.map(e=>`<option value="${d(e)}"></option>`).join(``)}
    </datalist>
  `}function Wi(e,t){let n=B.includes(e)?e:`kobold`;return`
    <select data-simple-backend-mode class="${t?`virtual-backend-select virtual-runtime-select`:``}">
      ${B.map(e=>`<option value="${d(e)}"${e===n?` selected`:``}>${u(dn[e])}</option>`).join(``)}
    </select>
  `}function Gi(e,t){let n=un.includes(e)?e:`none`,r=e&&e!==n?`<option value="${d(e)}" selected>${u(e)}</option>`:``;return`
    <select data-simple-field="${d(ln)}" class="${t?`virtual-runtime-select`:``}">
      ${r}
      ${un.map(e=>`<option value="${d(e)}"${e===n&&!r?` selected`:``}>${u(fn[e])}</option>`).join(``)}
    </select>
  `}function Ki(e){return`
    <div class="sidebar-value">
      <strong>${u(e.value)}</strong>
      <span class="muted">${u(e.config)}</span>
    </div>
  `}async function qi(e){try{let t=await e(Ji());return k.cookOutput.textContent=JSON.stringify(t,null,2),t}catch(e){return ea(e),null}}function Ji(){let e=W(),t=k.cookIdInput.value.trim(),n=!!(e&&t===e.local_id);return{node_id:r.simpleCook.nodeID,node_url:U()?.node_url||``,id:t,filename:e?.filename||``,overwrite:n||k.overwriteInput.checked,options:G(r.simpleCook.fields)}}function Yi(e){let t=Tn(e,b).map(e=>({...e,keys:e.keys.filter(e=>!Ci.includes(e))})).filter(e=>e.keys.length>0),n=t.find(e=>e.section===`runtime`);return n?(n.keys=[...Ci,...n.keys],t):[...t,{section:`runtime`,keys:[...Ci]}]}function Xi(e){return e===`backend_mode`&&!Object.hasOwn(r.simpleCook.fields,`backend_mode`)?Zi():e===`router_unload_policy`&&!Object.hasOwn(r.simpleCook.fields,`router_unload_policy`)?`none`:r.simpleCook.fields[e]}function Zi(){let e=r.simpleCook.fields[z];if(typeof e==`string`&&B.includes(e))return e;let t=W()?.backend_mode||U()?.backend_mode||`kobold`;return B.includes(t)?t:`kobold`}function Qi(e){r.simpleCook.mode=`edit`,r.simpleCook.configID=e?.local_id||``,r.simpleCook.fields=G(e?.options??{}),r.simpleCook.cleanFields=G(e?.options??{}),r.simpleCook.sidebar=null,r.simpleCook.openSections=[],k.cookIdInput.value=e?.local_id||Nn(U(),`new-config`)}function $i(e,t){let n=e.value;e.innerHTML=t.map(e=>`<option value="${d(e.value)}">${u(e.label)}</option>`).join(``),Array.from(e.options).some(e=>e.value===n)&&(e.value=n)}function ea(e){k.cookOutput.textContent=JSON.stringify(xe(e),null,2)}function ta(e){return e.section||`other`}function na(e){return e===1?`1 field`:`${e} fields`}function ra(){k.loginView.classList.remove(`hidden`),k.appView.classList.add(`hidden`)}function ia(){k.loginView.classList.add(`hidden`),k.appView.classList.remove(`hidden`)}function aa(){ca(),oa(),A(),Lt(),X(),q(),sa()}function Z(){let e=r.router;k.routerSummary.textContent=`${e?.url||``} ${e?.running?`running`:`stopped`}`,k.launchButton.disabled=!e?.managed||!!e?.running,k.restartButton.disabled=!e?.managed,k.shutdownButton.disabled=!e?.can_shutdown,k.forceKillButton.disabled=!e?.can_force_kill,k.routerStatus.innerHTML=[f(`Managed`,e?.managed?`yes`:`no`),f(`Running`,e?.running?`yes`:`no`),f(`URL`,e?.url||`unknown`),f(`PID`,e?.pid?String(e.pid):`none`),f(`Can shutdown`,e?.can_shutdown?`yes`:`no`),f(`Can force kill`,e?.can_force_kill?`yes`:`no`),f(`Last error`,e?.error||`none`)].join(``)}function oa(){let e=k.filterInput.value.trim().toLowerCase(),t=We(e),n=Ge(e);k.modelsTable.innerHTML=t.map(e=>`
    <tr>
      <td>${u(e.public_id||e.local_id)}</td>
      <td>${u(e.node_id||``)}</td>
      <td>${u(e.backend_mode||``)}</td>
      <td>${u(Me(e))}</td>
      <td>${u(Ne(e.options))}</td>
      <td>${u(Oe(e))}</td>
      <td>${e.available?`yes`:`no`}</td>
      <td>
        <button type="button" data-load-config="${d(e.public_id||e.local_id)}">Load</button>
      </td>
    </tr>
  `).join(``),k.filesTable.innerHTML=n.map(e=>`
    <tr>
      <td title="${d(e.path)}">${u(e.basename)}</td>
      <td>${u(e.node_id||``)}</td>
      <td>${u(h(e).join(`, `))}</td>
      <td>${Be(e.size||0)}</td>
    </tr>
  `).join(``)}function sa(){let e=r.inventory?.recipes??[];k.recipeCount.textContent=`${e.length} recipes`,k.recipesList.innerHTML=e.map(e=>`
    <article class="recipe-item">
      <div>
        <strong>${u(e.public_id||e.id)}</strong>
        <div class="muted">${u(e.public_image_id||``)}</div>
      </div>
      <button type="button" data-delete-recipe="${d(e.id)}">Delete</button>
    </article>
  `).join(``)}function ca(){let e=r.inventory?.nodes??[];k.nodeCount.textContent=`${e.length} nodes`,k.nodesGrid.innerHTML=e.map(e=>{let t=e.hardware;return`
      <article class="node-card">
        <strong>${u(e.node_id||e.node_url||`unknown`)}</strong>
        <div class="muted">${u(e.node_url||`local`)}</div>
        <div class="node-meta">
          ${p(e.backend_mode||`unknown`,`cyan`)}
          ${p(e.available?`available`:`down`,e.available?`lime`:`amber`)}
          ${p(`${t.max_threads||`?`} threads`,`magenta`)}
          ${p(`${t.gpu_backend||`unknown`} gpu`,`cyan`)}
        </div>
        ${e.error?`<div class="error-text">${u(e.error)}</div>`:``}
      </article>
    `}).join(``)}async function la(){try{r.csrf=(await c()).csrf,ia(),await ua()}catch{ra()}}async function ua(){await da(),await Q(),await J(),await I()}async function da(){r.router=await ne(),Z()}async function Q(){r.inventory=await se(),aa()}function fa(e){r.activeTab=e,O(`[data-tab]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.tab===e)),O(`[data-panel]`,HTMLElement).forEach(t=>t.classList.toggle(`active`,t.dataset.panel===e))}function pa(e){Sa(e)&&(r.activeCookMode=e,O(`[data-cook-mode]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.cookMode===e)),O(`[data-cook-panel]`,HTMLElement).forEach(t=>t.classList.toggle(`active`,t.dataset.cookPanel===e)))}function ma(e){Ca(e)&&(r.activePalette=e,O(`[data-palette]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.palette===e)),q())}O(`[data-tab]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>fa(e.dataset.tab||``))}),O(`[data-cook-mode]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>pa(e.dataset.cookMode))}),O(`[data-palette]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>ma(e.dataset.palette))}),k.loginForm.addEventListener(`submit`,e=>{e.preventDefault(),ha()}),k.logoutButton.addEventListener(`click`,()=>$(ga)),k.refreshButton.addEventListener(`click`,()=>$(ua)),k.webuiFilterInput.addEventListener(`input`,()=>li(k.webuiFilterInput.value)),k.webuiGrid.addEventListener(`click`,e=>{let t=E(e),n=t?.dataset.webuiOpen;if(n){di(n);return}let r=t?.dataset.webuiDetails;r&&fi(r)}),k.webuiGrid.addEventListener(`change`,e=>{let t=E(e),n=t?.dataset.webuiToggle;n&&t instanceof HTMLInputElement&&$(()=>ui(n,t.checked))}),k.filterInput.addEventListener(`input`,oa),k.modelsTable.addEventListener(`click`,e=>{let t=E(e)?.dataset.loadConfig;t&&$(()=>ti(t,Q))}),k.benchmarkModelSelect.addEventListener(`change`,()=>{ct(k.benchmarkModelSelect.value),$(ot)}),k.benchmarkTypeSelect.addEventListener(`change`,()=>lt(k.benchmarkTypeSelect.value)),k.benchmarkAllSections.addEventListener(`change`,()=>ut(k.benchmarkAllSections.checked)),k.benchmarkSections.addEventListener(`change`,dt),k.runBenchmarkButton.addEventListener(`click`,()=>$(async()=>{await st(),await Q()})),k.analyticsPeriodSelect.addEventListener(`change`,()=>$(async()=>{Rt(k.analyticsPeriodSelect.value),await I()})),k.analyticsNodeSelect.addEventListener(`change`,()=>$(async()=>{zt(k.analyticsNodeSelect.value),await I()})),k.analyticsModelSelect.addEventListener(`change`,()=>$(async()=>{Bt(k.analyticsModelSelect.value),await I()})),k.analyticsSectionSelect.addEventListener(`change`,()=>$(async()=>{Vt(k.analyticsSectionSelect.value),await I()})),k.analyticsRefreshButton.addEventListener(`click`,()=>$(I)),k.constructorFilterInput.addEventListener(`input`,q),k.launchButton.addEventListener(`click`,()=>$(_a)),k.restartButton.addEventListener(`click`,()=>$(va)),k.shutdownButton.addEventListener(`click`,()=>$(ya)),k.forceKillButton.addEventListener(`click`,()=>$(ba)),k.previewButton.addEventListener(`click`,()=>$(Pi)),k.cookForm.addEventListener(`submit`,e=>{e.preventDefault(),Fi(Q)}),k.simpleNodeSelect.addEventListener(`change`,()=>wi(k.simpleNodeSelect.value)),k.simpleConfigSelect.addEventListener(`change`,()=>Ti(k.simpleConfigSelect.value)),k.simpleFieldFilter.addEventListener(`input`,()=>Ei(k.simpleFieldFilter.value)),k.simpleAddFieldButton.addEventListener(`click`,Ai),k.simpleNewButton.addEventListener(`click`,Oi),k.simpleCopyButton.addEventListener(`click`,ki),k.simpleDeleteButton.addEventListener(`click`,()=>$(()=>Ii(Q))),k.simpleConfigEditor.addEventListener(`change`,e=>ji(e.target)),k.simpleConfigEditor.addEventListener(`toggle`,e=>Di(e.target),!0),k.simpleConfigEditor.addEventListener(`click`,e=>{let t=E(e),n=t?.dataset.fieldValues;if(n){Ni(n,`field`);return}let r=t?.dataset.fieldModelValues;if(r){Ni(r,`model`);return}let i=t?.dataset.removeSimpleField;i&&Mi(i)}),k.simpleFieldSidebar.addEventListener(`click`,e=>{E(e)?.dataset.closeFieldSidebar!==void 0&&(r.simpleCook.sidebar=null,X())}),k.advancedPreviewButton.addEventListener(`click`,()=>$(Qr)),k.advancedApplyButton.addEventListener(`click`,()=>$(()=>$r(Q))),k.clearConstructorButton.addEventListener(`click`,kr),k.advancedBackendSelect.addEventListener(`change`,()=>Ir(k.advancedBackendSelect.value)),k.paletteList.addEventListener(`dragstart`,e=>{if(!(e instanceof DragEvent))return;let t=D(e.target,`[data-drag-payload]`,HTMLElement)?.dataset.dragPayload;!t||!e.dataTransfer||(e.dataTransfer.setData(`text/plain`,t),e.dataTransfer.effectAllowed=`copy`)}),k.paletteList.addEventListener(`click`,e=>{let t=E(e),n=t?.dataset.addOption;if(n){Or(n);return}let i=t?.dataset.selectPayload;i&&Dr(r.palettePayloads[i])}),k.constructorLanes.addEventListener(`dragover`,e=>{let t=D(e.target,`[data-drop-lane]`,HTMLElement);t&&(e.preventDefault(),t.classList.add(`drag-over`))}),k.constructorLanes.addEventListener(`dragleave`,e=>{D(e.target,`[data-drop-lane]`,HTMLElement)?.classList.remove(`drag-over`)}),k.constructorLanes.addEventListener(`drop`,e=>{if(!(e instanceof DragEvent))return;let t=D(e.target,`[data-drop-lane]`,HTMLElement);!t||!e.dataTransfer||(e.preventDefault(),t.classList.remove(`drag-over`),Dr(r.palettePayloads[e.dataTransfer.getData(`text/plain`)],t.dataset.dropLane))}),k.constructorLanes.addEventListener(`click`,e=>{let t=E(e),n=t?.dataset.clearLane;if(n){Ar(n);return}let r=t?.dataset.editLaneFields;r&&jr(r)}),k.constructorLanes.addEventListener(`change`,e=>Fr(e.target)),k.constructorFieldDialog.addEventListener(`cancel`,e=>{e.preventDefault(),Jn()}),k.constructorFieldDialog.addEventListener(`click`,e=>{Xn(e.target,q)}),k.constructorFieldDialog.addEventListener(`change`,e=>{Yn(e.target)}),k.webuiDialog.addEventListener(`cancel`,e=>{e.preventDefault(),pi()}),k.webuiDialog.addEventListener(`click`,e=>{let t=E(e);if(t?.dataset.webuiDialogClose!==void 0){pi();return}let n=t?.dataset.webuiEnable;if(n){$(()=>ui(n,!0));return}let r=t?.dataset.webuiLoad;r&&$(()=>mi(r,t.dataset.webuiLoadModel||``,t.dataset.webuiLoadImage||``))}),k.selectedOptionsList.addEventListener(`input`,e=>Mr(e.target)),k.selectedOptionsList.addEventListener(`click`,e=>{let t=E(e),n=t?.dataset.removeOption;if(n){Nr(n);return}let r=t?.dataset.toggleList;r&&Pr(r)}),k.usedModelsList.addEventListener(`click`,e=>{let t=E(e)?.dataset.toggleList;t&&Pr(t)}),k.recipesList.addEventListener(`click`,e=>{xa(e)}),la();async function ha(){k.loginError.textContent=``;try{r.csrf=(await ee(k.tokenInput.value)).csrf,ia(),await ua()}catch(e){k.loginError.textContent=wa(e)}}async function ga(){await te(),r.csrf=``,ra()}async function _a(){r.router=await re(),Z(),await J()}async function va(){r.router=await ie(),Z(),await J()}async function ya(){r.router=await ae(),Z(),await J()}async function ba(){r.router=await oe(),Z(),await J()}async function xa(e){let t=E(e)?.dataset.deleteRecipe;t&&(await _e(t),await Q(),sa())}function $(e){e()}function Sa(e){return e===`quick`||e===`constructor`}function Ca(e){return e===`configs`||e===`files`||e===`options`}function wa(e){return e instanceof Error?e.message:String(e)}