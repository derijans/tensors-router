(function(){let e=document.createElement(`link`).relList;if(e&&e.supports&&e.supports(`modulepreload`))return;for(let e of document.querySelectorAll(`link[rel="modulepreload"]`))n(e);new MutationObserver(e=>{for(let t of e)if(t.type===`childList`)for(let e of t.addedNodes)e.tagName===`LINK`&&e.rel===`modulepreload`&&n(e)}).observe(document,{childList:!0,subtree:!0});function t(e){let t={};return e.integrity&&(t.integrity=e.integrity),e.referrerPolicy&&(t.referrerPolicy=e.referrerPolicy),e.crossOrigin===`use-credentials`?t.credentials=`include`:e.crossOrigin===`anonymous`?t.credentials=`omit`:t.credentials=`same-origin`,t}function n(e){if(e.ep)return;e.ep=!0;let n=t(e);fetch(e.href,n)}})();function e(){return{text:null,image:null,embeddings:null,voice:null,music:null}}function t(){return{text:``,image:``,embeddings:``,voice:``,music:``}}function n(){return{text:{},image:{},embeddings:{},voice:{},music:{}}}var r={csrf:``,inventory:null,router:null,benchmark:{modelKey:``,type:`general`,sections:[`runtime`,`llm`,`embed`,`image`,`voice`,`music`],record:null,running:!1,error:``},analytics:{query:{period:`24h`},data:null,loading:!1,error:``},webuis:{data:null,filter:``,loading:!1,error:``,action:``},activeTab:`router`,activeCookMode:`quick`,activePalette:`configs`,simpleCook:{nodeID:``,configID:``,fields:{},cleanFields:{},mode:`edit`,fieldFilter:``,openSections:[],sidebar:null},constructor:{lanes:e(),targetNodes:t(),laneOptions:n(),backendMode:`kobold`,backendTouched:!1,options:{},fieldEditor:null,fieldPresets:[],showUsedAll:!1,showOptionsAll:!1},palettePayloads:{}};function i(e){return typeof e!=`object`||!e||Array.isArray(e)?!1:Object.values(e).every(a)}function a(e){if(e===null)return!0;switch(typeof e){case`boolean`:case`number`:case`string`:return!0;case`object`:return Array.isArray(e)?e.every(a):i(e);default:return!1}}function o(e){return typeof e==`object`&&e&&!Array.isArray(e)?e:null}async function s(e,t={}){let n=new Headers(t.headers);t.body&&!n.has(`Content-Type`)&&n.set(`Content-Type`,`application/json`),r.csrf&&t.method&&t.method!==`GET`&&n.set(`X-CSRF-Token`,r.csrf);let i=await fetch(e,{...t,headers:n}),a=await i.text(),o=Ce(a);if(!i.ok)throw Te(we(o,a,i.statusText),o);return o}function c(){return s(`/api/session`)}function ee(e){return s(`/api/login`,{method:`POST`,body:JSON.stringify({token:e})})}function te(){return s(`/api/logout`,{method:`POST`})}function ne(){return s(`/api/router/status`)}function re(){return s(`/api/router/launch`,{method:`POST`})}function ie(){return s(`/api/router/restart`,{method:`POST`})}function ae(){return s(`/api/router/shutdown`,{method:`POST`})}function oe(){return s(`/api/router/force-kill`,{method:`POST`})}function se(){return s(`/api/inventory`)}function ce(){return s(`/api/webuis`)}function le(e){return s(`/api/webuis/session`,{method:`POST`,body:JSON.stringify(e)})}function ue(e){return s(`/api/webuis/load`,{method:`POST`,body:JSON.stringify(e)})}function de(e,t){let n=new URLSearchParams({model_id:t});return e&&n.set(`node_id`,e),s(`/api/benchmarks?${n.toString()}`)}function fe(e){return s(`/api/benchmarks/run`,{method:`POST`,body:JSON.stringify(e)})}function pe(e){let t=new URLSearchParams({period:e.period});return e.node_id&&t.set(`node_id`,e.node_id),e.model_id&&t.set(`model_id`,e.model_id),e.section&&t.set(`section`,e.section),s(`/api/analytics?${t.toString()}`)}function me(e){return s(`/api/load`,{method:`POST`,body:JSON.stringify(e)})}function he(e){return s(`/api/cook/preview`,{method:`POST`,body:JSON.stringify(e)})}function ge(e){return s(`/api/cook/apply`,{method:`POST`,body:JSON.stringify(e)})}function _e(e){return s(`/api/cook/${encodeURIComponent(e)}`,{method:`DELETE`})}function ve(e){return s(`/api/config-file/preview`,{method:`POST`,body:JSON.stringify(e)})}function ye(e){return s(`/api/config-file/apply`,{method:`POST`,body:JSON.stringify(e)})}function be(e){return s(`/api/config-file`,{method:`DELETE`,body:JSON.stringify(e)})}function xe(e){if(Se(e)){let t=Ee(o(e.data)?.validation);return t?{error:e.message,validation:t}:{error:e.message}}return{error:e instanceof Error?e.message:String(e)}}function Se(e){return e instanceof Error&&`data`in e}function Ce(e){if(!e)return null;try{return JSON.parse(e)}catch{return{raw:e}}}function we(e,t,n){let r=o(e);if(typeof r?.error==`string`)return r.error;let i=o(r?.error);return typeof i?.message==`string`?i.message:t||n}function Te(e,t){let n=Error(e);return n.data=t,n}function Ee(e){if(!Array.isArray(e))return null;let t=e.filter(De);return t.length>0?t:null}function De(e){let t=o(e);return typeof t?.severity==`string`&&typeof t.code==`string`&&typeof t.message==`string`}var l=[`runtime`,`llm`,`embed`,`image`,`voice`,`music`];function Oe(e){let t=e.benchmark?.latest;if(!t)return`none`;let n=ke(t,`tokens_per_second`);return n?.value?`${t.status} ${n.value.toFixed(1)} tok/s`:`${t.status} ${t.duration_ms||0}ms`}function ke(e,t){return e.metrics?.find(e=>e.name===t)??null}function u(e){let t={"&":`&amp;`,"<":`&lt;`,">":`&gt;`,'"':`&quot;`,"'":`&#39;`};return Be(e).replace(/[&<>"']/g,e=>t[e]??e)}function d(e){return u(e).replace(/`/g,`&#96;`)}function f(e,t){return`
    <div class="status-item">
      <div class="status-label">${u(e)}</div>
      <div class="status-value">${u(t)}</div>
    </div>
  `}function p(e,t){let n=Be(e).trim();return n?`<span class="chip ${d(t)}">${u(n)}</span>`:``}function Ae(e){return`
    <div class="issue ${e.severity===`error`?`error`:``}">
      <strong>${u(e.severity)} / ${u(e.code)}</strong>
      <span>${u(e.message)}</span>
    </div>
  `}function m(e,t,n,r){return{severity:e,code:t,message:n,field:r}}function h(e){switch(e){case`image`:return`magenta`;case`embeddings`:return`lime`;case`voice`:return`amber`;case`music`:return`violet`;default:return`cyan`}}function je(e){let t=[];return e.has_llm&&t.push(`llm`),e.has_image&&t.push(`image`),e.has_embeddings&&t.push(`embeddings`),e.has_multimodal&&t.push(`multimodal`),e.has_voice&&t.push(`voice`),e.has_music&&t.push(`music`),t.join(`, `)||`none`}function Me(e){let t=Object.keys(e??{}).length;return t?`${t} filled`:`none`}function g(e){return e.roles??[e.role||`unknown`]}function Ne(e,t){return e.some(e=>e.kind===t)}function Pe(e){let t=String(e).toLowerCase();return[`gpulayers`,`tensor_split`,`maingpu`,`usecuda`,`usecublas`,`embeddingsgpu`,`sdclipgpu`,`sdflashattention`].includes(e)||t.includes(`gpu`)||t.includes(`cuda`)}function Fe(e){return typeof e==`boolean`?e:typeof e==`number`?e!==0:typeof e==`string`?e.trim()!==``:e!=null}function Ie(e){if(typeof e==`number`)return e;if(typeof e==`string`){let t=Number.parseInt(e,10);return Number.isFinite(t)?t:0}return 0}function Le(e){return typeof e==`string`?e:e===void 0?``:JSON.stringify(e)??``}function _(e){return typeof e==`string`?e:e===void 0?``:JSON.stringify(e)??``}function v(e,t){let n=t.trim();switch(e?.value_type){case`bool`:return n===`true`||n===`1`||n===`yes`;case`number`:{let e=Number(n);return Number.isFinite(e)?e:0}case`json`:if(!n)return{};try{let e=JSON.parse(n);return a(e)?e:t}catch{return t}default:return t}}function y(e){return e==null?!0:typeof e==`string`?e.trim()===``:Array.isArray(e)?e.length===0||e.every(y):typeof e==`object`?Object.keys(e).length===0:!1}function Re(e){return typeof e==`string`?e.trim():JSON.stringify(e)??``}function ze(e){return e<1024?`${e} B`:e<1024*1024?`${(e/1024).toFixed(1)} KB`:e<1024*1024*1024?`${(e/1024/1024).toFixed(1)} MB`:`${(e/1024/1024/1024).toFixed(1)} GB`}function Be(e){return e==null?``:typeof e==`string`?e:typeof e==`number`||typeof e==`boolean`||typeof e==`bigint`?e.toString():JSON.stringify(e)??``}function Ve(){return(r.inventory?.nodes??[]).flatMap(e=>e.models??[])}function He(){return(r.inventory?.nodes??[]).flatMap(e=>e.files??[])}function b(){return[...r.inventory?.option_catalog??[],...r.inventory?.observed_options??[]]}function x(e){return b().find(t=>t.key===e)}function S(e){return(r.inventory?.nodes??[]).find(t=>t.node_id===e)}function Ue(e){let t=r.inventory?.models?.length?r.inventory.models:Ve();return e?t.filter(t=>JSON.stringify(t).toLowerCase().includes(e)):t}function We(e){let t=He();return e?t.filter(t=>JSON.stringify(t).toLowerCase().includes(e)):t}function Ge(e){let t=new Map;for(let n of e){let e=n.node_id||r.inventory?.node_id||`local`,i=t.get(e)??[];i.push(n),t.set(e,i)}return t}function Ke(e,t,n){let r=[];for(let[i,a]of Object.entries(qe(e,t,n))){if(!$e(i))continue;let e=Ie(a);e>0&&r.push({key:i,value:e})}return r.sort((e,t)=>e.key.localeCompare(t.key))}function qe(e,t,n){let i={};for(let n of t){let t=r.constructor.lanes[n.kind],a=r.constructor.targetNodes[n.kind]||t?.component.node_id||``;!t||(a||``)!==(e||``)||(Object.assign(i,t.model?.options??{}),Object.assign(i,r.constructor.laneOptions[n.kind]??{}))}return Object.assign(i,n),i}function Je(){let e={};for(let t of rt())Object.assign(e,t.model?.options??{}),Object.assign(e,r.constructor.laneOptions[t.lane]??{});return Object.assign(e,r.constructor.options),e}function Ye(e){let t=e.model?.options??{},n=[];for(let e of[`model_param`,`model`,`sdmodel`,`embeddingsmodel`,`mmproj`,`sdvae`,`sdt5xxl`,`sdclipl`,`sdclipg`,`sdupscaler`,`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`,`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]){let r=t[e];if(typeof r==`string`&&r.trim())n.push(`${e}: ${r}`);else if(Array.isArray(r))for(let t of r)typeof t==`string`&&t.trim()&&n.push(`${e}: ${t}`)}return e.file?.path&&n.push(`file: ${e.file.path}`),n}function Xe(){return Ve().flatMap(e=>{let t=[];return e.has_llm&&t.push(C(`text`,e)),e.has_image&&t.push(C(`image`,e)),e.has_embeddings&&t.push(C(`embeddings`,e)),e.has_voice&&t.push(C(`voice`,e)),e.has_music&&t.push(C(`music`,e)),t})}function Ze(){return He().flatMap(e=>{let t=[];return g(e).includes(`llm`)&&t.push(w(`text`,e)),g(e).includes(`image`)&&t.push(w(`image`,e)),g(e).includes(`embeddings`)&&t.push(w(`embeddings`,e)),g(e).includes(`voice`)&&t.push(w(`voice`,e)),g(e).includes(`music`)&&t.push(w(`music`,e)),t})}function Qe(){return b().map(e=>({title:e.name||e.key,subtitle:e.key,badge:e.lane||`option`,color:e.known?`cyan`:`amber`,meta:[e.value_type||`json`,...e.backends??[],e.native_flag??``,e.known?`known`:`observed`].filter(T),payload:{type:`option`,key:e.key}}))}function $e(e){let t=x(e);return t?t.value_type===`number`&&t.key.endsWith(`threads`):String(e||``).trim().toLowerCase().endsWith(`threads`)}function C(e,t){let n=e===`image`?t.public_image_id||t.image_id||t.local_id:t.public_id||t.local_id;return{title:n,subtitle:t.filename||``,badge:e,color:h(e),meta:[t.node_id||``,t.backend_mode||``,nt(t.options)].filter(T),payload:{type:`component`,lane:e,label:n,subtitle:t.filename||``,meta:[t.node_id||``,t.backend_mode||``].filter(T),component:et(e,t),model:t}}}function w(e,t){return{title:t.basename,subtitle:t.path,badge:e,color:h(e),meta:[t.node_id||``,ze(t.size||0)].filter(T),payload:{type:`component`,lane:e,label:t.basename,subtitle:t.path,meta:[t.node_id||``,`file`].filter(T),component:tt(e,t),file:t}}}function et(e,t){let n={kind:e,node_id:t.node_id,node_url:t.node_url||``,source:`config`,model_id:t.local_id};return e===`image`&&(n.image_id=t.image_id||``),n}function tt(e,t){return{kind:e,node_id:t.node_id,source:`file`,file_path:t.path}}function nt(e){let t=Object.keys(e??{}).length;return t?`${t} options`:``}function rt(){return Object.values(r.constructor.lanes).filter(e=>e!==null)}function T(e){return e.trim()!==``}function it(e,t){return{value:e,label:t}}function E(e,t){let n=document.getElementById(e);if(!(n instanceof t))throw Error(`Expected #${e} to be ${t.name}`);return n}function D(e){return e.target instanceof HTMLElement?e.target:null}function at(e,t,n){if(!(e instanceof Element))return null;let r=e.closest(t);return r instanceof n?r:null}function O(e,t){return Array.from(document.querySelectorAll(e)).filter(e=>e instanceof t)}var k={loginView:E(`loginView`,HTMLElement),appView:E(`appView`,HTMLElement),loginForm:E(`loginForm`,HTMLFormElement),tokenInput:E(`tokenInput`,HTMLInputElement),loginError:E(`loginError`,HTMLElement),logoutButton:E(`logoutButton`,HTMLButtonElement),refreshButton:E(`refreshButton`,HTMLButtonElement),launchButton:E(`launchButton`,HTMLButtonElement),restartButton:E(`restartButton`,HTMLButtonElement),shutdownButton:E(`shutdownButton`,HTMLButtonElement),forceKillButton:E(`forceKillButton`,HTMLButtonElement),routerSummary:E(`routerSummary`,HTMLElement),routerStatus:E(`routerStatus`,HTMLElement),nodeCount:E(`nodeCount`,HTMLElement),nodesGrid:E(`nodesGrid`,HTMLElement),webuiFilterInput:E(`webuiFilterInput`,HTMLInputElement),webuiStatus:E(`webuiStatus`,HTMLElement),webuiGrid:E(`webuiGrid`,HTMLElement),filterInput:E(`filterInput`,HTMLInputElement),modelsActionStatus:E(`modelsActionStatus`,HTMLElement),modelsTable:E(`modelsTable`,HTMLTableSectionElement),filesTable:E(`filesTable`,HTMLTableSectionElement),benchmarkModelSelect:E(`benchmarkModelSelect`,HTMLSelectElement),benchmarkTypeSelect:E(`benchmarkTypeSelect`,HTMLSelectElement),benchmarkAllSections:E(`benchmarkAllSections`,HTMLInputElement),benchmarkSections:E(`benchmarkSections`,HTMLElement),runBenchmarkButton:E(`runBenchmarkButton`,HTMLButtonElement),benchmarkLatest:E(`benchmarkLatest`,HTMLElement),benchmarkHistory:E(`benchmarkHistory`,HTMLElement),analyticsPeriodSelect:E(`analyticsPeriodSelect`,HTMLSelectElement),analyticsNodeSelect:E(`analyticsNodeSelect`,HTMLSelectElement),analyticsModelSelect:E(`analyticsModelSelect`,HTMLSelectElement),analyticsSectionSelect:E(`analyticsSectionSelect`,HTMLSelectElement),analyticsRefreshButton:E(`analyticsRefreshButton`,HTMLButtonElement),analyticsStatus:E(`analyticsStatus`,HTMLElement),analyticsSummary:E(`analyticsSummary`,HTMLElement),analyticsTimeline:E(`analyticsTimeline`,HTMLElement),analyticsSections:E(`analyticsSections`,HTMLElement),analyticsModelsTable:E(`analyticsModelsTable`,HTMLTableSectionElement),analyticsNodesTable:E(`analyticsNodesTable`,HTMLTableSectionElement),analyticsRecentTable:E(`analyticsRecentTable`,HTMLTableSectionElement),analyticsNodeErrors:E(`analyticsNodeErrors`,HTMLElement),cookForm:E(`cookForm`,HTMLFormElement),cookIdInput:E(`cookIdInput`,HTMLInputElement),overwriteInput:E(`overwriteInput`,HTMLInputElement),simpleNodeSelect:E(`simpleNodeSelect`,HTMLSelectElement),simpleConfigSelect:E(`simpleConfigSelect`,HTMLSelectElement),simpleFieldFilter:E(`simpleFieldFilter`,HTMLInputElement),simpleAddFieldSelect:E(`simpleAddFieldSelect`,HTMLSelectElement),simpleAddFieldButton:E(`simpleAddFieldButton`,HTMLButtonElement),simpleNewButton:E(`simpleNewButton`,HTMLButtonElement),simpleCopyButton:E(`simpleCopyButton`,HTMLButtonElement),simpleDeleteButton:E(`simpleDeleteButton`,HTMLButtonElement),simpleConfigEditor:E(`simpleConfigEditor`,HTMLElement),simpleFieldSidebar:E(`simpleFieldSidebar`,HTMLElement),previewButton:E(`previewButton`,HTMLButtonElement),cookOutput:E(`cookOutput`,HTMLPreElement),recipeCount:E(`recipeCount`,HTMLElement),recipesList:E(`recipesList`,HTMLElement),advancedBackendSelect:E(`advancedBackendSelect`,HTMLSelectElement),advancedCookIdInput:E(`advancedCookIdInput`,HTMLInputElement),constructorFilterInput:E(`constructorFilterInput`,HTMLInputElement),clearConstructorButton:E(`clearConstructorButton`,HTMLButtonElement),advancedPreviewButton:E(`advancedPreviewButton`,HTMLButtonElement),advancedApplyButton:E(`advancedApplyButton`,HTMLButtonElement),paletteList:E(`paletteList`,HTMLElement),constructorLanes:E(`constructorLanes`,HTMLElement),validationList:E(`validationList`,HTMLElement),usedModelsList:E(`usedModelsList`,HTMLElement),selectedOptionsList:E(`selectedOptionsList`,HTMLElement),constructorFieldDialog:E(`constructorFieldDialog`,HTMLDialogElement),constructorFieldDialogBody:E(`constructorFieldDialogBody`,HTMLElement),webuiDialog:E(`webuiDialog`,HTMLDialogElement),webuiDialogBody:E(`webuiDialogBody`,HTMLElement)};function A(){yt(),k.benchmarkModelSelect.innerHTML=bt().map(e=>`
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
  `).join(``)}function vt(){if(r.benchmark.record)return r.benchmark.record;let e=j();if(!e?.benchmark)return null;let t={node_id:e.node_id,model_id:e.local_id,history:[]};return e.benchmark.latest&&(t.latest=e.benchmark.latest),e.benchmark.sections&&(t.sections=e.benchmark.sections),t}function yt(){r.benchmark.modelKey&&j()||(r.benchmark.modelKey=xt(bt()[0]))}function j(){return bt().find(e=>xt(e)===r.benchmark.modelKey)??null}function bt(){return Ve()}function xt(e){return e?`${e.node_id}\n${e.local_id}`:``}function St(e){return`${e.node_id||`node`} / ${e.local_id||e.public_id}`}function Ct(){return r.benchmark.sections.length===l.length}function wt(e){return l.includes(e)}function Tt(e){return e?new Date(e).toLocaleString():`never`}var Et=[{value:`24h`,label:`Last 24 hours`},{value:`7d`,label:`Last 7 days`},{value:`30d`,label:`Last 30 days`},{value:`90d`,label:`Last 90 days`},{value:`all`,label:`All time`}],Dt=[{value:``,label:`All sections`},{value:`llm`,label:`LLM`},{value:`embed`,label:`Embeddings`},{value:`image`,label:`Images`},{value:`voice`,label:`Voice`},{value:`music`,label:`Music`}];function Ot(e){return[{value:``,label:`All nodes`},...Ft((e?.nodes??[]).map(e=>e.node_id).filter(It)).map(e=>({value:e,label:e}))]}function kt(e){return[{value:``,label:`All models`},...Ft((e?.nodes??[]).flatMap(e=>e.models??[]).map(e=>e.local_id||e.public_id).filter(It)).map(e=>({value:e,label:e}))]}function At(e){let t={period:e.period||`24h`};return e.node_id&&(t.node_id=e.node_id),e.model_id&&(t.model_id=e.model_id),e.section&&(t.section=e.section),t}function M(e){return Math.round(Number.isFinite(e??0)?e??0:0).toLocaleString(`en-US`)}function N(e,t=1){let n=Number.isFinite(e??0)?e??0:0;return Number.isInteger(n)?n.toLocaleString(`en-US`):n.toLocaleString(`en-US`,{maximumFractionDigits:t,minimumFractionDigits:n>0&&n<10?t:0})}function jt(e){let t=Number.isFinite(e??0)?e??0:0;if(t<60)return`${N(t,1)}s`;let n=t/60;return n<60?`${N(n,1)}m`:`${N(n/60,1)}h`}function P(e){return`${M(e)} MB`}function F(e){return`${N(e,1)}%`}function Mt(e,t,n){let r={points:[],linePath:``,ticks:[]};if(e.length===0||t<=0||n<=0)return r;let i=Math.max(...e.map(e=>e.request_count),1),a=Math.max(0,t-8),o=Math.max(0,n-8),s=e.length-1,c=e.map((e,t)=>({x:4+(s===0?.5:t/s)*a,y:4+(1-e.request_count/i)*o,radius:4}));return{points:c,linePath:c.map((e,t)=>`${t===0?`M`:`L`} ${e.x.toFixed(2)} ${e.y.toFixed(2)}`).join(` `),ticks:Nt(e,c)}}function Nt(e,t){if(e.length===0||t.length===0)return[];let n=e.length-1,r=n<=3?e.map((e,t)=>t):[0,Math.round(n/3),Math.round(n*2/3),n];return Array.from(new Set(r)).map(n=>({x:t[n]?.x??0,label:Pt(e[n]?.bucket_start)}))}function Pt(e){return e?new Date(e).toLocaleDateString(`en-US`,{month:`short`,day:`numeric`}):``}function Ft(e){return Array.from(new Set(e)).sort((e,t)=>e.localeCompare(t))}function It(e){return!!e?.trim()}async function I(){r.analytics.loading=!0,r.analytics.error=``,Lt();try{r.analytics.data=await pe(At(r.analytics.query))}catch(e){r.analytics.error=e instanceof Error?e.message:String(e)}finally{r.analytics.loading=!1,Lt()}}function Lt(){Ht();let e=r.analytics.data;r.analytics.error?k.analyticsStatus.innerHTML=`<div class="error-text">${u(r.analytics.error)}</div>`:r.analytics.loading?k.analyticsStatus.innerHTML=`<div class="detail-empty">Loading analytics</div>`:e?.enabled?k.analyticsStatus.innerHTML=``:k.analyticsStatus.innerHTML=`<div class="detail-empty">Analytics disabled</div>`,Ut(),Wt(),Gt(),Kt()}function Rt(e){rn(e)&&(r.analytics.query.period=e)}function zt(e){e?r.analytics.query.node_id=e:delete r.analytics.query.node_id}function Bt(e){e?r.analytics.query.model_id=e:delete r.analytics.query.model_id}function Vt(e){e?r.analytics.query.section=e:delete r.analytics.query.section}function Ht(){let e=At(r.analytics.query);k.analyticsPeriodSelect.innerHTML=R(Et,e.period),k.analyticsNodeSelect.innerHTML=R(nn(Ot(r.inventory),e.node_id),e.node_id||``),k.analyticsModelSelect.innerHTML=R(nn(kt(r.inventory),e.model_id),e.model_id||``),k.analyticsSectionSelect.innerHTML=R(Dt,e.section||``)}function Ut(){let e=r.analytics.data?.summary;if(!r.analytics.data?.enabled||!e){k.analyticsSummary.innerHTML=``;return}k.analyticsSummary.innerHTML=[L(`Requests`,M(e.request_count),`${M(e.success_count)} ok / ${M(e.failure_count)} failed`),L(`Tokens`,M(e.total_tokens),`${M(e.input_tokens)} in / ${M(e.output_tokens)} out`),L(`Speed`,`${N(e.average_tokens_per_second,1)} tok/s`,`${N(e.average_duration_ms,0)}ms avg`),L(`Images`,M(e.image_count),`generated or returned`),L(`Audio`,jt(e.audio_seconds),`${M(e.audio_tokens)} tokens`),L(`VRAM`,P(e.vram_peak_mb),`${F(e.vram_peak_percent)} peak / ${P(e.vram_total_mb)} total`),L(`Loads`,M(e.load_count),`${N(e.average_load_duration_ms,0)}ms avg / ${P(e.model_vram_estimate_mb)} model`)].join(``)}function Wt(){let e=r.analytics.data?.timeline??[];if(!r.analytics.data?.enabled||e.length===0){k.analyticsTimeline.innerHTML=``;return}let t=Mt(e,720,170);k.analyticsTimeline.innerHTML=`
    <div class="analytics-chart-head">
      <strong>Timeline</strong>
      <span class="muted">${u(r.analytics.data.granularity)}</span>
    </div>
    <svg class="analytics-chart" viewBox="0 0 720 220" role="img" aria-label="Analytics timeline">
      <path class="analytics-line" d="${d(t.linePath)}"></path>
      ${t.points.map((t,n)=>{let r=e[n];return r?`
        <circle class="analytics-point" cx="${t.x.toFixed(2)}" cy="${t.y.toFixed(2)}" r="${t.radius.toFixed(2)}">
          <title>${u(on(r))}: ${M(r.request_count)} requests</title>
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
      <span>${u(an(e.section))}</span>
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
      <td>${P(e.vram_peak_mb)} / ${F(e.vram_peak_percent)}</td>
      <td>${M(e.total_tokens)}</td>
      <td>${M(e.image_count)}</td>
      <td>${jt(e.audio_seconds)}</td>
    </tr>
  `}function Yt(e){return`
    <tr>
      <td>${u(e.node_id)}</td>
      <td>${M(e.request_count)}</td>
      <td>${M(e.load_count)}</td>
      <td>${P(e.vram_peak_mb)} / ${F(e.vram_peak_percent)}</td>
      <td>${M(e.total_tokens)}</td>
      <td>${M(e.image_count)}</td>
      <td>${jt(e.audio_seconds)}</td>
    </tr>
  `}function Xt(e){let t=e.event_type===`model_load`?en(e):e.section===`image`?Qt(e):e.section===`voice`||e.section===`music`?$t(e):Zt(e);return`
    <tr>
      <td>${u(sn(e.finished_at))}</td>
      <td>${u(e.node_id)}</td>
      <td>${u(e.model_id||`unknown`)}</td>
      <td>${u(an(e.section))}</td>
      <td>${u(e.backend_mode||``)}</td>
      <td>${u(e.success?`ok`:String(e.status_code))}</td>
      <td>${u(t)}</td>
    </tr>
  `}function Zt(e){let t=e.tokens_per_second?` / ${N(e.tokens_per_second,1)} tok/s`:``;return`${M(e.input_tokens)} in / ${M(e.output_tokens)} out${t}${tn(e)}`}function Qt(e){let t=e.image_width&&e.image_height?` / ${e.image_width}x${e.image_height}`:``;return`${e.image_type?`${e.image_type} / `:``}${M(e.image_count)} images${t}${tn(e)}`}function $t(e){return`${jt(e.audio_seconds)} / ${M(e.audio_tokens)} tokens${tn(e)}`}function en(e){return`${e.config_filename?`${e.config_filename} / `:``}${N(e.duration_ms,0)}ms / ${P(e.load_vram_before_mb)} -> ${P(e.load_vram_after_mb)} / +${P(e.load_vram_delta_mb)}`}function tn(e){return!e.work_vram_max_mb&&!e.model_vram_estimate_mb?``:` / VRAM ${P(e.work_vram_max_mb)} (${F(e.vram_peak_percent)}) / model ${P(e.model_vram_estimate_mb)}`}function R(e,t){return e.map(e=>`
    <option value="${d(e.value)}" ${e.value===t?`selected`:``}>${u(e.label)}</option>
  `).join(``)}function nn(e,t){return!t||e.some(e=>e.value===t)?e:[...e,{value:t,label:t}]}function rn(e){return e===`24h`||e===`7d`||e===`30d`||e===`90d`||e===`all`}function an(e){return Dt.find(t=>t.value===e)?.label??e}function on(e){return sn(e.bucket_start)}function sn(e){return e?new Date(e).toLocaleString():`never`}var z=[`text`,`image`,`embeddings`,`voice`,`music`],B=`backend_mode`,V=[`kobold`,`llama_sdcpp`];[...z],[...z];var cn={kobold:`Kobold`,llama_sdcpp:`llama/sd.cpp`},H={text:{label:`LLM`,shortLabel:`Text`,section:`llm`,accent:`cyan`,dropLabel:`Drop a text config or model file`},image:{label:`Image`,shortLabel:`Image`,section:`image`,accent:`magenta`,dropLabel:`Drop an image config or model file`},embeddings:{label:`Embed`,shortLabel:`Embed`,section:`embed`,accent:`lime`,dropLabel:`Drop an embedding config or model file`},voice:{label:`Voice`,shortLabel:`Voice`,section:`voice`,accent:`amber`,dropLabel:`Drop Whisper, TTS, tokenizer, or voice dir`},music:{label:`Music`,shortLabel:`Music`,section:`music`,accent:`violet`,dropLabel:`Drop Music LLM, embeddings, diffusion, or VAE`}};function ln(e){return z.includes(e)}var un={voice:[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],music:[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]};function dn(e){return e===`voice`||e===`music`?un[e]:[]}function fn(e,t){return(t===`voice`||t===`music`)&&e.component.source===`file`&&!e.component.option_key}function pn(e,t){return e!==`voice`&&e!==`music`?!1:un[e].includes(t)}function mn(e,t){let n={};for(let[r,i]of Object.entries(e))_n(i)!==_n(t[r])&&(n[r]=i);return n}function hn(e){return JSON.parse(JSON.stringify(e||{}))}function gn(e){return`${e.backendMode}\n${e.section}\n${e.name}`}function _n(e){return JSON.stringify(e??null)??``}var vn=[`llm`,`image`,`embed`,`voice`,`music`,`runtime`,`other`],yn={llm:`LLM`,image:`Image`,embed:`Embed`,voice:`Voice`,music:`Music`,runtime:`Runtime`,other:`Other`},bn={llm:[`model_param`,`model`],image:[`sdmodel`],embed:[`embeddingsmodel`,`mmproj`],voice:[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],music:[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]};function U(){return S(r.simpleCook.nodeID)??(r.inventory?.nodes??[])[0]??null}function W(){return(U()?.models??[]).find(e=>e.local_id===r.simpleCook.configID)??null}function xn(){let e=U(),t=e?.node_id||``,n=e?.models??[];return{node:e,nodeFiles:He().filter(e=>e.node_id===t),nodeModels:n,otherNodeModels:n.filter(e=>e.local_id!==r.simpleCook.configID),comparableBySection:new Map}}function Sn(e,t){let n=new Map(vn.map(e=>[e,[]]));for(let r of Object.keys(e).sort((e,t)=>e.localeCompare(t))){let e=Fn(t(r)),i=n.get(e)??[];i.push(r),n.set(e,i)}return vn.map(e=>({section:e,keys:n.get(e)??[]})).filter(e=>e.keys.length>0)}function Cn(e,t,n){return Bn([...t?.choices??[],...Mn(t,n),...Nn(e,n)].map(e=>zn(e,t)))}function wn(e,t,n){let i=r.simpleCook.fields[e],a=In(r.simpleCook.fields,t),o=Pn(t,n),s=o.map(t=>t.options?.[e]).filter(e=>!y(e));if(s.length===0)return a&&o.length===0&&!y(i)?`compare-same`:`compare-none`;let c=Re(i);return s.every(e=>Re(e)===c)?`compare-same`:`compare-different`}function Tn(e,t,n,r){let i=Fn(n(e)),a=t===`model`?Pn(i,r):r.otherNodeModels,o=[],s=new Set;for(let t of a){let n=t.options?.[e];if(y(n))continue;let r=_(n),i=`${r}\n${t.local_id}`;s.has(i)||(s.add(i),o.push({value:r,config:kn(t)}))}return o}function En(e){let t=e?.hardware,n={quiet:!0,nomodel:!1,contextsize:4096,threads:t?.max_threads?Math.max(1,Math.floor(t.max_threads/2)):-1,batchsize:512,usemmap:!0,usemlock:!1,gpulayers:t?.gpu_backend&&t.gpu_backend!==`cpu`&&t.gpu_backend!==`unknown`?`auto`:`0`};(t?.gpu_backend===`cuda`||t?.gpu_backend===`rocm`)&&(n.usecuda=!0),t?.gpu_backend===`vulkan`&&(n.usevulkan=!0);let r=Vn(e?.node_url||``);return r&&(n.host=r.hostname,r.port&&(n.port=Number(r.port))),n}function Dn(e){if(e?.default!==void 0&&e.default!==``)return v(e,e.default);switch(e?.value_type){case`bool`:return!1;case`number`:return 0;case`json`:return{};default:return``}}function G(e){return JSON.parse(JSON.stringify(e||{}))}function On(e){return`${e.node_id||`node`} / ${e.backend_mode||`backend`}`}function kn(e){return`${e.local_id||e.public_id||`config`} / ${e.filename||``}`}function An(e,t){return`${(e?.node_id||`node`).toLowerCase().replace(/[^a-z0-9_-]+/g,`-`).replace(/^-|-$/g,``)||`node`}-${t}`}function jn(e){return String(e).replace(/[^a-z0-9_-]/gi,`-`)}function Mn(e,t){if(!e?.model_role)return[];let n=t.nodeFiles.filter(t=>Ln(g(t),e.model_role??``)).map(e=>e.path),r=t.nodeModels.flatMap(t=>Rn(t,e.model_role??``));return[...n,...r]}function Nn(e,t){return t.nodeModels.map(t=>t.options?.[e]).filter(e=>!y(e)).map(_)}function Pn(e,t){let n=t.comparableBySection.get(e);if(n)return n;let i=In(r.simpleCook.fields,e),a=t.otherNodeModels;return i?(a=a.filter(t=>In(t.options??{},e)===i),t.comparableBySection.set(e,a),a):(t.comparableBySection.set(e,a),a)}function Fn(e){return e?.section||`other`}function In(e,t){for(let n of bn[t]??[]){let t=e?.[n];if(!y(t))return Re(t)}return``}function Ln(e,t){return t===`llm`?e.includes(`llm`):t===`image`?e.includes(`image`):t===`embeddings`?e.includes(`embeddings`)||e.includes(`llm`):t===`multimodal`?e.includes(`multimodal`):t===`vae`?e.includes(`vae`):t===`clip`?e.includes(`clip`):t===`t5`?e.includes(`t5`):t===`upscaler`?e.includes(`upscaler`):t===`lora`?e.includes(`lora`):t===`voice`?e.includes(`voice`):t===`music`?e.includes(`music`):!0}function Rn(e,t){let n=e.capabilities??{},r=[];return t===`llm`&&typeof e.filename==`string`&&r.push(e.filename),t===`image`&&n.image?.model&&r.push(n.image.model),t===`embeddings`&&n.embeddings?.model&&r.push(n.embeddings.model),t===`multimodal`&&n.multimodal?.projector&&r.push(n.multimodal.projector),t===`vae`&&n.image?.vae&&r.push(n.image.vae),t===`clip`&&r.push(n.image?.clip1,n.image?.clip2,n.image?.clip_l,n.image?.clip_g),t===`t5`&&n.image?.t5xxl&&r.push(n.image.t5xxl),t===`upscaler`&&n.image?.upscaler&&r.push(n.image.upscaler),t===`lora`&&r.push(...n.image?.lora??[]),t===`voice`&&r.push(n.voice?.whisper_model,n.voice?.tts_model,n.voice?.wav_tokenizer,n.voice?.directory),t===`music`&&r.push(n.music?.llm,n.music?.embeddings,n.music?.diffusion,n.music?.vae),r.filter(e=>!!e)}function zn(e,t){if(t?.value_type===`json`)try{return JSON.parse(e),e}catch{return JSON.stringify(e)}return e}function Bn(e){let t=new Set,n=[];for(let r of e){let e=String(r??``).trim();!e||t.has(e)||(t.add(e),n.push(e))}return n}function Vn(e){try{return new URL(e)}catch{return null}}var Hn=`tensors-router.constructorFieldPresets`;function Un(){if(!(r.constructor.fieldPresets.length>0))try{let e=JSON.parse(window.localStorage.getItem(Hn)||`[]`);r.constructor.fieldPresets=Array.isArray(e)?e.filter(pr):[]}catch{r.constructor.fieldPresets=[]}}function Wn(e,t){Un(),r.constructor.fieldEditor={lane:e,draft:hn(r.constructor.laneOptions[e])},t&&(r.constructor.fieldEditor.pendingPayload=t),K(),fr()}function Gn(){r.constructor.fieldEditor=null,k.constructorFieldDialog.close(),k.constructorFieldDialogBody.innerHTML=``}function K(){let e=r.constructor.fieldEditor;if(!e){k.constructorFieldDialog.open&&Gn();return}let t=e.lane,n=H[t],i=or(e.pendingPayload??r.constructor.lanes[t]),a=nr(t,i,e.draft);k.constructorFieldDialogBody.innerHTML=`
    <div class="field-dialog-head">
      <div>
        <h3>${u(n.label)} Fields</h3>
        <p class="muted">${u(n.section)} staged overrides</p>
      </div>
      <button class="icon-button" type="button" title="Close" data-field-modal-action="cancel">x</button>
    </div>
    ${e.pendingPayload?tr(t,e.pendingPayload):``}
    <div class="preset-row">
      <label>
        Preset
        <select data-field-preset-select>${ir(t)}</select>
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
        <select data-field-add-select>${rr(t,a)}</select>
      </label>
      <button type="button" data-field-modal-action="add-field">Add Field</button>
    </div>
    <div class="field-diff-grid">
      ${a.map(t=>er(t,i[t],e.draft)).join(``)||`<div class="detail-empty">No fields in this section</div>`}
    </div>
    <div class="field-dialog-actions">
      <button type="button" data-field-modal-action="reset-section">Reset Section</button>
      <span></span>
      <button type="button" data-field-modal-action="cancel">Cancel</button>
      <button type="button" data-field-modal-action="apply">Apply</button>
    </div>
  `}function Kn(e){let t=r.constructor.fieldEditor;if(!t||!(e instanceof HTMLInputElement))return;let n=e.dataset.fieldDraft;if(n)try{t.draft[n]=v(x(n),e.value),e.setCustomValidity(``),K()}catch{e.setCustomValidity(`Invalid JSON`),e.reportValidity()}}function qn(e,t){let n=e instanceof HTMLElement?e.closest(`[data-field-modal-action]`):null;if(!(n instanceof HTMLElement))return;let r=n.dataset.fieldModalAction;if(r===`cancel`){Gn();return}if(r===`apply`){Jn(),t();return}if(r===`reset-section`){Yn();return}if(r===`reset-field`){Xn(n.dataset.fieldKey||``);return}if(r===`add-field`){Zn();return}if(r===`apply-preset`){Qn();return}r===`save-preset`&&$n()}function Jn(){let e=r.constructor.fieldEditor;if(!e)return;if(e.pendingPayload){let t=cr();if(!pn(e.lane,t)){k.constructorFieldDialogBody.querySelector(`[data-file-option-key]`)?.setAttribute(`aria-invalid`,`true`);return}r.constructor.lanes[e.lane]=lr(e.pendingPayload,t)}let t=or(r.constructor.lanes[e.lane]);r.constructor.laneOptions[e.lane]=mn(e.draft,t),Gn()}function Yn(){let e=r.constructor.fieldEditor;e&&(e.draft={},K())}function Xn(e){let t=r.constructor.fieldEditor;t&&(delete t.draft[e],K())}function Zn(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-add-select]`);!e||!(t instanceof HTMLSelectElement)||!t.value||(e.draft[t.value]=Dn(x(t.value)),K())}function Qn(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-preset-select]`);if(!e||!(t instanceof HTMLSelectElement)||!t.value)return;let n=ar(e.lane).find(e=>gn(e)===t.value);n&&(Object.assign(e.draft,hn(n.values)),K())}function $n(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-preset-name]`);if(!e||!(t instanceof HTMLInputElement)||!t.value.trim())return;let n={name:t.value.trim(),backendMode:sr(e),section:H[e.lane].section,values:hn(e.draft)};r.constructor.fieldPresets=[...r.constructor.fieldPresets.filter(e=>gn(e)!==gn(n)),n],window.localStorage.setItem(Hn,JSON.stringify(r.constructor.fieldPresets)),K()}function er(e,t,n){let r=x(e),i=Object.hasOwn(n,e),a=i?n[e]:void 0,o=i&&_n(a)!==_n(t);return`
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
        <input data-field-draft="${d(e)}" value="${d(i?Le(a):``)}" placeholder="inherit">
      </label>
      <div class="field-state">
        ${i?p(o?`changed`:`same`,o?`amber`:`violet`):p(`source`,``)}
        <button class="icon-button" type="button" title="Reset field" data-field-modal-action="reset-field" data-field-key="${d(e)}">x</button>
      </div>
    </div>
  `}function tr(e,t){if(!fn(t,e))return``;let n=dn(e);return`
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
  `}function nr(e,t,n){let r=H[e].section,i=new Set;for(let e of b())(e.section||`other`)===r&&i.add(e.key);for(let a of[...Object.keys(t),...Object.keys(n),...ur(e)]){let e=x(a);(!e||(e.section||`other`)===r)&&i.add(a)}return Array.from(i).sort((e,t)=>e.localeCompare(t))}function rr(e,t){let n=new Set(t),r=H[e].section;return b().filter(e=>(e.section||`other`)===r&&!n.has(e.key)).sort(dr).map(e=>`<option value="${d(e.key)}">${u(e.key)}</option>`).join(``)}function ir(e){return ar(e).map(e=>`<option value="${d(gn(e))}">${u(e.name)}</option>`).join(``)}function ar(e){let t=r.constructor.fieldEditor,n=H[e].section,i=t?sr(t):``;return r.constructor.fieldPresets.filter(e=>e.section===n&&(!e.backendMode||e.backendMode===i))}function or(e){return hn(e?.model?.options??{})}function sr(e){let t=e.pendingPayload??r.constructor.lanes[e.lane];return t?.model?.backend_mode?t.model.backend_mode:S(t?.component.node_id||``)?.backend_mode||`unknown`}function cr(){let e=k.constructorFieldDialogBody.querySelector(`[data-file-option-key]`);return e instanceof HTMLSelectElement?e.value:``}function lr(e,t){return{...e,component:{...e.component,option_key:t}}}function ur(e){return dn(e)}function dr(e,t){return e.key.localeCompare(t.key)}function fr(){k.constructorFieldDialog.open||k.constructorFieldDialog.showModal()}function pr(e){if(!e||typeof e!=`object`)return!1;let t=e;return typeof t.name==`string`&&typeof t.backendMode==`string`&&typeof t.section==`string`&&!!t.values&&typeof t.values==`object`&&!Array.isArray(t.values)}function mr(e,t){let n=hr(e),r={};for(let[e,i]of Object.entries(t)){let t=x(e);(!t||t.section===`runtime`||t.section===n)&&(r[e]=i)}return r}function hr(e){return H[e].section}function gr(){let e=Object.entries(r.constructor.lanes).filter(e=>e[1]!==null),t=e.map(([e,t])=>yr(e,t)),n={};for(let[t,i]of e)Object.assign(n,mr(t,i.model?.options??{})),Object.assign(n,r.constructor.laneOptions[t]??{});return Object.assign(n,r.constructor.options),r.constructor.backendTouched&&(n[B]=r.constructor.backendMode),{id:k.advancedCookIdInput.value.trim(),overwrite:k.overwriteInput.checked,components:t,options:n}}function _r(){let e=[],t=gr();t.id||e.push(m(`warning`,`id_missing`,`Config id is empty.`,`id`)),t.components.length===0&&e.push(m(`warning`,`empty_constructor`,`No lanes selected.`,``));for(let[n,r]of Ge(t.components)){let i=S(n),a=qe(n,r,t.options??{}),o=vr(a,i?.backend_mode||`kobold`);o===`kobold`&&Ne(r,`image`)&&Ne(r,`embeddings`)&&e.push(m(`error`,`kobold_image_embeddings_mix`,`Kobold cannot cook image and embeddings into the same config.`,n));let s=i?.hardware?.max_threads||0;for(let i of Ke(n,r,t.options??{}))s>0&&i.value>s&&e.push(m(`error`,`thread_budget_exceeded`,`${i.key} uses ${i.value} threads on a node with ${s} logical CPUs.`,i.key));if(i?.hardware?.gpu_backend===`rocm`)for(let[t,n]of Object.entries(a))x(t)?.cuda_only&&Fe(n)&&e.push(m(`error`,`cuda_on_rocm`,`${t} is CUDA-only on a ROCm node.`,t));if(!i?.hardware?.gpu_backend||i.hardware.gpu_backend===`unknown`){for(let[t,r]of Object.entries(a))if(Pe(t)&&Fe(r)){e.push(m(`warning`,`gpu_backend_unknown`,`GPU backend could not be inferred.`,n));break}}for(let[t]of Object.entries(a)){let n=x(t);n?.known&&(n.backends?.length??0)>0&&!(n.backends??[]).includes(o)&&e.push(m(`warning`,`unsupported_option`,`${t} is not marked as supported by ${o}.`,t))}}return e}function vr(e,t){let n=e[B];return typeof n==`string`&&V.includes(n)?n:V.includes(t)?t:`kobold`}function yr(e,t){let n=r.constructor.targetNodes[e]||t.component.node_id||``,i=S(n),a={...t.component,node_id:n,node_url:i?.node_url||t.component.node_url||``};if(n&&t.component.node_id&&n!==t.component.node_id){let n=br(e,t);n.path&&(a.source=`file`,a.file_path=n.path,n.optionKey?a.option_key=n.optionKey:delete a.option_key,delete a.model_id,delete a.image_id)}return a}function br(e,t){let n=t.model?.options??{};return e===`image`?{path:Sr(n.sdmodel)||t.file?.path||``}:e===`embeddings`?{path:Sr(n.embeddingsmodel)||t.file?.path||``}:e===`voice`?xr(n,[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],t.file?.path):e===`music`?xr(n,[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`],t.file?.path):{path:Sr(n.model_param)||Cr(n.model)||t.file?.path||``}}function xr(e,t,n){for(let n of t){let t=Sr(e[n]);if(t)return{path:t,optionKey:n}}let r=n||``;if(!r)return{path:``};let i=t[0];return i?{path:r,optionKey:i}:{path:r}}function Sr(e){return typeof e==`string`?e.trim():``}function Cr(e){if(typeof e==`string`)return e.trim();if(Array.isArray(e)){for(let t of e)if(typeof t==`string`&&t.trim())return t.trim()}return``}function q(){Pr(),Fr(),Ir(),Lr(),K()}function wr(e,t){if(!e)return;if(e.type===`option`){Tr(e.key);return}let n=ln(t)?t:e.lane;if(n===e.lane){if(fn(e,n)){Wn(n,e);return}r.constructor.lanes[n]=e,q()}}function Tr(e){let t=x(e);t&&(Object.hasOwn(r.constructor.options,e)||(r.constructor.options[e]=Kr(t)),q())}function Er(){r.constructor.lanes=e(),r.constructor.targetNodes=t(),r.constructor.laneOptions=n(),r.constructor.backendMode=`kobold`,r.constructor.backendTouched=!1,r.constructor.options={},r.constructor.fieldEditor=null,q()}function Dr(e){ln(e)&&(r.constructor.lanes[e]=null,r.constructor.laneOptions[e]={},q())}function Or(e){!ln(e)||!r.constructor.lanes[e]||Wn(e)}function kr(e){if(!(e instanceof HTMLInputElement))return;let t=e.dataset.optionInput;if(t)try{r.constructor.options[t]=v(x(t),e.value),e.setCustomValidity(``),Rr()}catch{e.setCustomValidity(`Invalid JSON`),e.reportValidity()}}function Ar(e){delete r.constructor.options[e],q()}function jr(e){e===`used`&&(r.constructor.showUsedAll=!r.constructor.showUsedAll),e===`options`&&(r.constructor.showOptionsAll=!r.constructor.showOptionsAll),Lr()}function Mr(e){if(!(e instanceof HTMLSelectElement))return;let t=e.dataset.laneTarget;ln(t)&&(r.constructor.targetNodes[t]=e.value,q())}function Nr(e){V.includes(e)&&(r.constructor.backendMode=e,r.constructor.backendTouched=!0,q())}function Pr(){let e=Jr();k.advancedBackendSelect.innerHTML=V.map(t=>{let n=t===e?` selected`:``;return`<option value="${d(t)}"${n}>${u(cn[t])}</option>`}).join(``),k.advancedBackendSelect.classList.toggle(`virtual-backend-select`,!r.constructor.backendTouched)}function Fr(){let e=k.constructorFilterInput.value.trim().toLowerCase(),t=zr().filter(t=>!e||JSON.stringify(t).toLowerCase().includes(e));r.palettePayloads={},k.paletteList.innerHTML=t.map(e=>{let t=`payload-${Object.keys(r.palettePayloads).length}`;r.palettePayloads[t]=e.payload;let n=e.payload.type===`option`?`<button type="button" data-add-option="${d(e.payload.key)}">Add</button>`:`<button type="button" data-select-payload="${d(t)}">Use</button>`;return`
      <article class="palette-item" draggable="true" data-drag-payload="${d(t)}">
        <div class="palette-title">
          <strong>${u(e.title)}</strong>
          ${p(e.badge,e.color)}
        </div>
        <div class="muted">${u(e.subtitle)}</div>
        <div class="palette-meta">${e.meta.map(e=>p(e,``)).join(``)}</div>
        ${n}
      </article>
    `}).join(``)||`<div class="detail-empty">No items</div>`}function Ir(){k.constructorLanes.innerHTML=z.map(Hr).join(``);for(let e of z){let t=document.querySelector(`[data-drop-lane="${e}"]`);if(!(t instanceof HTMLElement))continue;let n=r.constructor.lanes[e];if(!n){t.innerHTML=`<div class="lane-empty">${u(H[e].dropLabel)}</div>`;continue}let i=Object.keys(r.constructor.laneOptions[e]??{}).length;t.innerHTML=`
      <article class="selected-card">
        <strong>${u(n.label)}</strong>
        <div class="muted">${u(n.subtitle)}</div>
        <div class="palette-meta">${n.meta.map(e=>p(e,``)).join(``)}</div>
        ${n.component.option_key?`<div class="muted">Assigned to ${u(n.component.option_key)}</div>`:``}
        <label>
          Target node
          <select data-lane-target="${d(e)}">${qr(e,n)}</select>
        </label>
        <div class="lane-card-actions">
          <button type="button" data-edit-lane-fields="${d(e)}">Edit fields</button>
          ${i?p(`${i} overrides`,H[e].accent):``}
        </div>
      </article>
    `}}function Lr(){Rr();let e=Br();k.usedModelsList.innerHTML=Gr(e,r.constructor.showUsedAll,`used`).join(``)||`<div class="detail-empty">No models selected</div>`;let t=Vr();k.selectedOptionsList.innerHTML=Gr(t,r.constructor.showOptionsAll,`options`).join(``)||`<div class="detail-empty">No options selected</div>`}function Rr(){let e=_r();k.validationList.innerHTML=e.length?e.map(Ae).join(``):`<div class="detail-empty">Clean</div>`}function zr(){return r.activePalette===`files`?Ze():r.activePalette===`options`?Qe():Xe()}function Br(){let e=[];for(let t of z){let n=r.constructor.lanes[t];if(n){e.push(`
      <div class="used-row">
        ${p(H[t].shortLabel,h(t))}
        <span>${u(n.label)}</span>
      </div>
    `);for(let t of Ye(n))e.push(`<div class="muted">${u(t)}</div>`)}}return e}function Vr(){let e=[],t=Je();for(let[n,i]of Object.entries(t).sort(([e],[t])=>e.localeCompare(t)))if(Object.hasOwn(r.constructor.options,n))e.push(Wr(n,r.constructor.options[n]));else if(Ur(n)){let t=Ur(n);e.push(`
        <div class="option-row">
          ${p(n,``)}
          ${t?p(`${H[t].shortLabel} override`,H[t].accent):``}
          <span class="muted">${u(_(i))}</span>
        </div>
      `)}else e.push(`
        <div class="option-row">
          ${p(n,``)}
          <span class="muted">${u(_(i))}</span>
        </div>
      `);return e}function Hr(e){let t=H[e];return`
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
  `}function Ur(e){return z.find(t=>Object.hasOwn(r.constructor.laneOptions[t]??{},e))??null}function Wr(e,t){return`
    <div class="option-editor">
      <span>${u(e)}</span>
      <input data-option-input="${d(e)}" value="${d(Le(t))}">
      <button type="button" data-remove-option="${d(e)}">Remove</button>
    </div>
  `}function Gr(e,t,n){return e.length<=9||t?e.length>9?[...e,`<button class="link-button" type="button" data-toggle-list="${n}">Show less</button>`]:e:[...e.slice(0,9),`<button class="link-button" type="button" data-toggle-list="${n}">Show all ${e.length}</button>`]}function Kr(e){switch(e.value_type){case`bool`:return!1;case`number`:return 0;case`json`:return{};default:return``}}function qr(e,t){let n=r.inventory?.nodes??[],i=r.constructor.targetNodes[e]||t.component.node_id||n[0]?.node_id||``;return r.constructor.targetNodes[e]||(r.constructor.targetNodes[e]=i),n.map(e=>{let t=e.node_id===i?` selected`:``;return`<option value="${d(e.node_id)}"${t}>${u(e.node_id||`node`)}</option>`}).join(``)}function Jr(){if(r.constructor.backendTouched&&V.includes(r.constructor.backendMode))return r.constructor.backendMode;for(let e of z){let t=r.constructor.lanes[e]?.model?.options?.[B];if(typeof t==`string`&&V.includes(t))return t}for(let e of z){let t=r.constructor.lanes[e];if(!t)continue;let n=S(r.constructor.targetNodes[e]||t.component.node_id||``)?.backend_mode||``;if(V.includes(n))return n}let e=r.inventory?.nodes?.[0]?.backend_mode||`kobold`;return V.includes(e)?e:`kobold`}async function Yr(){await Zr(he,gr())}async function Xr(e){let t=_r().filter(e=>e.severity===`error`);if(t.length>0){q(),k.cookOutput.textContent=JSON.stringify({validation:t},null,2);return}await Zr(ge,gr()),await e()}async function Zr(e,t){try{let n=await e(t);k.cookOutput.textContent=JSON.stringify(n,null,2),q()}catch(e){k.cookOutput.textContent=JSON.stringify(xe(e),null,2),q()}}async function Qr(e,t){let n=e.trim();if(n){$r(`Loading ${n}...`,!1);try{await me({model:n}),$r(`Loaded ${n}`,!1),await t()}catch(e){$r(e instanceof Error?e.message:String(e),!0)}}}function $r(e,t){k.modelsActionStatus.textContent=e,k.modelsActionStatus.classList.toggle(`error-text`,t)}function ei(e,t){let n=t.trim().toLowerCase();return n?e.filter(e=>[e.name,e.backend,e.backend_mode,e.lane,e.node_id,e.url,...e.compatible_models.map(e=>e.id)].join(` `).toLowerCase().includes(n)):e}function ti(e){let t=new Map;for(let n of e){let e=n.node_id||`local`;t.set(e,[...t.get(e)??[],n])}return Array.from(t.entries()).sort(([e],[t])=>e.localeCompare(t)).map(([e,t])=>({nodeID:e,entries:[...t].sort((e,t)=>e.name.localeCompare(t.name))}))}function ni(e){return e.enabled?e.requires_loaded_model&&!e.can_open_without_model&&!e.active?{openable:!1,reason:`needs_model`}:{openable:!0,reason:``}:{openable:!1,reason:`disabled`}}function ri(e){let t=ni(e);return{title:e.name,message:ai(t.reason),canEnable:!e.enabled,canLoad:e.compatible_models.length>0,models:ii(e)}}function ii(e){return[...e.compatible_models].sort((e,t)=>e.active===t.active?e.id.localeCompare(t.id):e.active?-1:1)}function ai(e){switch(e){case`disabled`:return`Enable this WebUI before opening.`;case`needs_model`:return`Load a compatible model before opening.`;default:return`Ready to open.`}}async function J(){r.webuis.loading=!0,r.webuis.error=``,Y();try{r.webuis.data=await ce()}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{r.webuis.loading=!1,Y()}}function oi(e){r.webuis.filter=e,Y()}async function si(e,t){r.webuis.action=t?`Enabled`:`Disabled`,r.webuis.error=``,Y();try{r.webuis.data=await le({id:e,enabled:t})}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{Y()}}function ci(e){let t=vi(e);if(t){if(!ni(t).openable){mi(t);return}yi(t.url)}}function li(e){let t=vi(e);t&&mi(t)}function ui(){k.webuiDialog.close()}async function di(e,t,n){let i=vi(e);if(i){r.webuis.action=`Loading ${t||n||i.name}...`,r.webuis.error=``,Y();try{let a=await ue({id:e,model_id:t,image_id:n});if(await J(),vi(e)?.enabled&&a.url){ui(),yi(a.url);return}r.webuis.action=`Loaded ${a.model_id||a.image_id||i.name}`}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{Y()}}}function Y(){let e=ei(r.webuis.data?.data??[],r.webuis.filter);k.webuiStatus.textContent=gi(e.length),k.webuiStatus.classList.toggle(`error-text`,r.webuis.error!==``),k.webuiGrid.innerHTML=e.length?ti(e).map(fi).join(``):`<div class="detail-empty">No WebUIs</div>`}function fi(e){return`
    <section class="webui-node-group">
      <div class="webui-node-head">
        <h3>${u(e.nodeID)}</h3>
        <span class="pill">${e.entries.length} WebUIs</span>
      </div>
      <div class="webui-cards">
        ${e.entries.map(pi).join(``)}
      </div>
    </section>
  `}function pi(e){let t=ni(e);return`
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
        ${p(e.lane,h(e.lane))}
        ${p(e.active?`active`:`idle`,e.active?`lime`:`amber`)}
      </div>
      <div class="webui-model-summary">${u(_i(e))}</div>
      <div class="webui-actions">
        <button type="button" data-webui-open="${d(e.id)}">Open</button>
        <button type="button" data-webui-details="${d(e.id)}">${t.openable?`Models`:`Resolve`}</button>
      </div>
    </article>
  `}function mi(e){let t=ri(e);k.webuiDialogBody.innerHTML=`
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
      ${t.canLoad?t.models.map(t=>hi(e,t)).join(``):`<div class="detail-empty">No compatible models</div>`}
    </div>
  `,k.webuiDialog.showModal()}function hi(e,t){return`
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
  `}function gi(e){return r.webuis.error?r.webuis.error:r.webuis.loading?`Loading...`:r.webuis.action||`${e} WebUIs`}function _i(e){let t=e.active_image_id||e.active_model_id;return t?`Active: ${t}`:`${e.compatible_models.length} compatible`}function vi(e){return(r.webuis.data?.data??[]).find(t=>t.id===e)}function yi(e){let t=window.open(e,`_blank`,`noopener,noreferrer`);t&&(t.opener=null)}function X(){Ni(),Pi(),Fi(),Ii(),Li()}function bi(e){r.simpleCook.nodeID=e,Ki((U()?.models??[])[0]??null),X()}function xi(e){Ki((U()?.models??[]).find(t=>t.local_id===e)??null),X()}function Si(e){r.simpleCook.fieldFilter=e,Ii()}function Ci(e){if(!(e instanceof HTMLDetailsElement))return;let t=e.dataset.simpleSection;if(!t)return;let n=new Set(r.simpleCook.openSections);e.open?n.add(t):n.delete(t),r.simpleCook.openSections=Array.from(n)}function wi(){let e=U();r.simpleCook.configID=``,r.simpleCook.mode=`new`,r.simpleCook.fields=En(e),r.simpleCook.cleanFields={},r.simpleCook.openSections=[],k.cookIdInput.value=An(e,`new-config`),X()}function Ti(){let e=W();r.simpleCook.mode=`copy`,r.simpleCook.configID=``,r.simpleCook.fields=G(r.simpleCook.fields),r.simpleCook.cleanFields={},r.simpleCook.openSections=[],k.cookIdInput.value=An(U(),`${e?.local_id||`config`}-copy`),X()}function Ei(){let e=k.simpleAddFieldSelect.value;if(!e||Object.hasOwn(r.simpleCook.fields,e))return;let t=x(e);r.simpleCook.fields[e]=Dn(t),X()}function Di(e){if(!(e instanceof HTMLInputElement)&&!(e instanceof HTMLSelectElement))return;if(e instanceof HTMLSelectElement&&e.dataset.simpleBackendMode!==void 0){r.simpleCook.fields[B]=e.value,X();return}let t=e.dataset.simpleField;t&&(r.simpleCook.fields[t]=v(x(t),e.value),X())}function Oi(e){delete r.simpleCook.fields[e],r.simpleCook.sidebar?.key===e&&(r.simpleCook.sidebar=null),X()}function ki(e,t){r.simpleCook.sidebar={key:e,type:t},Li()}async function Ai(){await Vi(ve)}async function ji(e){let t=await Vi(ye);t&&(r.simpleCook.mode=`edit`,r.simpleCook.configID=t.id||``,r.simpleCook.fields=G(t.options??r.simpleCook.fields),r.simpleCook.cleanFields=G(r.simpleCook.fields),await e())}async function Mi(e){let t=W();if(t&&window.confirm(`Delete ${t.filename||t.local_id}?`))try{await be({node_id:r.simpleCook.nodeID,node_url:U()?.node_url||``,id:t.local_id,filename:t.filename,overwrite:!1,options:{}}),await e()}catch(e){Ji(e)}}function Ni(){let e=r.inventory?.nodes??[];if(e.length===0){r.simpleCook.nodeID=``,r.simpleCook.configID=``,r.simpleCook.fields={},r.simpleCook.cleanFields={},r.simpleCook.openSections=[];return}if(!e.some(e=>e.node_id===r.simpleCook.nodeID)){let t=e[0];r.simpleCook.nodeID=t?.node_id??``,Ki((t?.models??[])[0]??null);return}if(r.simpleCook.mode!==`edit`)return;let t=W(),n=U();!t&&(n?.models??[]).length>0&&Ki((n?.models??[])[0]??null)}function Pi(){let e=r.inventory?.nodes??[];qi(k.simpleNodeSelect,e.map(e=>it(e.node_id,On(e)))),k.simpleNodeSelect.value=r.simpleCook.nodeID;let t=U()?.models??[];qi(k.simpleConfigSelect,t.map(e=>it(e.local_id,kn(e)))),k.simpleConfigSelect.value=r.simpleCook.configID,k.simpleConfigSelect.disabled=t.length===0,k.simpleCopyButton.disabled=Object.keys(r.simpleCook.fields||{}).length===0,k.simpleDeleteButton.disabled=!W(),k.simpleFieldFilter.value=r.simpleCook.fieldFilter}function Fi(){let e=r.simpleCook.fields||{},t=b().filter(t=>t.key!==`backend_mode`&&!Object.hasOwn(e,t.key)).sort((e,t)=>`${Yi(e)}:${e.key}`.localeCompare(`${Yi(t)}:${t.key}`));k.simpleAddFieldSelect.innerHTML=t.map(e=>{let t=`${yn[Yi(e)]||`Other`} / ${e.key}`;return`<option value="${d(e.key)}">${u(t)}</option>`}).join(``)}function Ii(){let e=r.simpleCook.fields||{},t=r.simpleCook.fieldFilter.trim().toLowerCase(),n=xn(),i=new Set(r.simpleCook.openSections),a=Ui(e).map(r=>{let a=r.keys.filter(e=>!t||`${e} ${_(Wi(e))}`.toLowerCase().includes(t)),o=a.map(t=>Ri(t,Wi(t),r.section,n,t===`backend_mode`&&!Object.hasOwn(e,`backend_mode`))).join(``);if(!o)return null;let s=yn[r.section]||r.section;return{section:r.section,html:`
        <details class="config-section" data-simple-section="${d(r.section)}"${i.has(r.section)?` open`:``}>
          <summary>
            <span>${u(s)}</span>
            <span class="section-count">${u(Xi(a.length))}</span>
          </summary>
          <div class="config-fields">${o}</div>
        </details>
      `}}).filter(e=>e!==null);k.simpleConfigEditor.innerHTML=a.length?a.map(e=>e.html).join(``):`<div class="detail-empty">No fields</div>`}function Li(){let e=r.simpleCook.sidebar;if(!e){k.simpleFieldSidebar.innerHTML=`<div class="detail-empty">Field values</div>`;return}let t=Tn(e.key,e.type,x,xn());k.simpleFieldSidebar.innerHTML=`
    <div class="field-sidebar-head">
      <div>
        <h3>${u(e.key)}</h3>
        <p class="muted">${u(e.type===`model`?`same model file`:`same field`)}</p>
      </div>
      <button type="button" data-close-field-sidebar>x</button>
    </div>
    <div class="detail-list">
      ${t.length?t.map(Bi).join(``):`<div class="detail-empty">No values</div>`}
    </div>
  `}function Ri(e,t,n,r,i=!1){let a=x(e),o=`field-values-${jn(e)}`,s=Cn(e,a,r),c=wn(e,n,r),ee=e===`backend_mode`?zi(Gi(),i):`
        <input data-simple-field="${d(e)}" list="${d(o)}" value="${d(Le(t))}">
        <datalist id="${d(o)}">
          ${s.map(e=>`<option value="${d(e)}"></option>`).join(``)}
        </datalist>
      `,te=bn[n]?`<button class="icon-button" type="button" title="Same model values" data-field-model-values="${d(e)}">M</button>`:``;return`
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
  `}function zi(e,t){let n=V.includes(e)?e:`kobold`;return`
    <select data-simple-backend-mode class="${t?`virtual-backend-select`:``}">
      ${V.map(e=>`<option value="${d(e)}"${e===n?` selected`:``}>${u(cn[e])}</option>`).join(``)}
    </select>
  `}function Bi(e){return`
    <div class="sidebar-value">
      <strong>${u(e.value)}</strong>
      <span class="muted">${u(e.config)}</span>
    </div>
  `}async function Vi(e){try{let t=await e(Hi());return k.cookOutput.textContent=JSON.stringify(t,null,2),t}catch(e){return Ji(e),null}}function Hi(){let e=W(),t=k.cookIdInput.value.trim(),n=!!(e&&t===e.local_id);return{node_id:r.simpleCook.nodeID,node_url:U()?.node_url||``,id:t,filename:e?.filename||``,overwrite:n||k.overwriteInput.checked,options:G(r.simpleCook.fields)}}function Ui(e){let t=Sn(e,x);if(Object.hasOwn(e,`backend_mode`))return t;let n=t.find(e=>e.section===`runtime`);return n?(n.keys=[B,...n.keys],t):[...t,{section:`runtime`,keys:[B]}]}function Wi(e){return e===`backend_mode`&&!Object.hasOwn(r.simpleCook.fields,`backend_mode`)?Gi():r.simpleCook.fields[e]}function Gi(){let e=r.simpleCook.fields[B];if(typeof e==`string`&&V.includes(e))return e;let t=W()?.backend_mode||U()?.backend_mode||`kobold`;return V.includes(t)?t:`kobold`}function Ki(e){r.simpleCook.mode=`edit`,r.simpleCook.configID=e?.local_id||``,r.simpleCook.fields=G(e?.options??{}),r.simpleCook.cleanFields=G(e?.options??{}),r.simpleCook.sidebar=null,r.simpleCook.openSections=[],k.cookIdInput.value=e?.local_id||An(U(),`new-config`)}function qi(e,t){let n=e.value;e.innerHTML=t.map(e=>`<option value="${d(e.value)}">${u(e.label)}</option>`).join(``),Array.from(e.options).some(e=>e.value===n)&&(e.value=n)}function Ji(e){k.cookOutput.textContent=JSON.stringify(xe(e),null,2)}function Yi(e){return e.section||`other`}function Xi(e){return e===1?`1 field`:`${e} fields`}function Zi(){k.loginView.classList.remove(`hidden`),k.appView.classList.add(`hidden`)}function Qi(){k.loginView.classList.add(`hidden`),k.appView.classList.remove(`hidden`)}function $i(){na(),ea(),A(),Lt(),X(),q(),ta()}function Z(){let e=r.router;k.routerSummary.textContent=`${e?.url||``} ${e?.running?`running`:`stopped`}`,k.launchButton.disabled=!e?.managed||!!e?.running,k.restartButton.disabled=!e?.managed,k.shutdownButton.disabled=!e?.can_shutdown,k.forceKillButton.disabled=!e?.can_force_kill,k.routerStatus.innerHTML=[f(`Managed`,e?.managed?`yes`:`no`),f(`Running`,e?.running?`yes`:`no`),f(`URL`,e?.url||`unknown`),f(`PID`,e?.pid?String(e.pid):`none`),f(`Can shutdown`,e?.can_shutdown?`yes`:`no`),f(`Can force kill`,e?.can_force_kill?`yes`:`no`),f(`Last error`,e?.error||`none`)].join(``)}function ea(){let e=k.filterInput.value.trim().toLowerCase(),t=Ue(e),n=We(e);k.modelsTable.innerHTML=t.map(e=>`
    <tr>
      <td>${u(e.public_id||e.local_id)}</td>
      <td>${u(e.node_id||``)}</td>
      <td>${u(e.backend_mode||``)}</td>
      <td>${u(je(e))}</td>
      <td>${u(Me(e.options))}</td>
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
      <td>${u(g(e).join(`, `))}</td>
      <td>${ze(e.size||0)}</td>
    </tr>
  `).join(``)}function ta(){let e=r.inventory?.recipes??[];k.recipeCount.textContent=`${e.length} recipes`,k.recipesList.innerHTML=e.map(e=>`
    <article class="recipe-item">
      <div>
        <strong>${u(e.public_id||e.id)}</strong>
        <div class="muted">${u(e.public_image_id||``)}</div>
      </div>
      <button type="button" data-delete-recipe="${d(e.id)}">Delete</button>
    </article>
  `).join(``)}function na(){let e=r.inventory?.nodes??[];k.nodeCount.textContent=`${e.length} nodes`,k.nodesGrid.innerHTML=e.map(e=>{let t=e.hardware;return`
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
    `}).join(``)}async function ra(){try{r.csrf=(await c()).csrf,Qi(),await ia()}catch{Zi()}}async function ia(){await aa(),await Q(),await J(),await I()}async function aa(){r.router=await ne(),Z()}async function Q(){r.inventory=await se(),$i()}function oa(e){r.activeTab=e,O(`[data-tab]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.tab===e)),O(`[data-panel]`,HTMLElement).forEach(t=>t.classList.toggle(`active`,t.dataset.panel===e))}function sa(e){ga(e)&&(r.activeCookMode=e,O(`[data-cook-mode]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.cookMode===e)),O(`[data-cook-panel]`,HTMLElement).forEach(t=>t.classList.toggle(`active`,t.dataset.cookPanel===e)))}function ca(e){_a(e)&&(r.activePalette=e,O(`[data-palette]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.palette===e)),q())}O(`[data-tab]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>oa(e.dataset.tab||``))}),O(`[data-cook-mode]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>sa(e.dataset.cookMode))}),O(`[data-palette]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>ca(e.dataset.palette))}),k.loginForm.addEventListener(`submit`,e=>{e.preventDefault(),la()}),k.logoutButton.addEventListener(`click`,()=>$(ua)),k.refreshButton.addEventListener(`click`,()=>$(ia)),k.webuiFilterInput.addEventListener(`input`,()=>oi(k.webuiFilterInput.value)),k.webuiGrid.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.webuiOpen;if(n){ci(n);return}let r=t?.dataset.webuiDetails;r&&li(r)}),k.webuiGrid.addEventListener(`change`,e=>{let t=D(e),n=t?.dataset.webuiToggle;n&&t instanceof HTMLInputElement&&$(()=>si(n,t.checked))}),k.filterInput.addEventListener(`input`,ea),k.modelsTable.addEventListener(`click`,e=>{let t=D(e)?.dataset.loadConfig;t&&$(()=>Qr(t,Q))}),k.benchmarkModelSelect.addEventListener(`change`,()=>{ct(k.benchmarkModelSelect.value),$(ot)}),k.benchmarkTypeSelect.addEventListener(`change`,()=>lt(k.benchmarkTypeSelect.value)),k.benchmarkAllSections.addEventListener(`change`,()=>ut(k.benchmarkAllSections.checked)),k.benchmarkSections.addEventListener(`change`,dt),k.runBenchmarkButton.addEventListener(`click`,()=>$(async()=>{await st(),await Q()})),k.analyticsPeriodSelect.addEventListener(`change`,()=>$(async()=>{Rt(k.analyticsPeriodSelect.value),await I()})),k.analyticsNodeSelect.addEventListener(`change`,()=>$(async()=>{zt(k.analyticsNodeSelect.value),await I()})),k.analyticsModelSelect.addEventListener(`change`,()=>$(async()=>{Bt(k.analyticsModelSelect.value),await I()})),k.analyticsSectionSelect.addEventListener(`change`,()=>$(async()=>{Vt(k.analyticsSectionSelect.value),await I()})),k.analyticsRefreshButton.addEventListener(`click`,()=>$(I)),k.constructorFilterInput.addEventListener(`input`,q),k.launchButton.addEventListener(`click`,()=>$(da)),k.restartButton.addEventListener(`click`,()=>$(fa)),k.shutdownButton.addEventListener(`click`,()=>$(pa)),k.forceKillButton.addEventListener(`click`,()=>$(ma)),k.previewButton.addEventListener(`click`,()=>$(Ai)),k.cookForm.addEventListener(`submit`,e=>{e.preventDefault(),ji(Q)}),k.simpleNodeSelect.addEventListener(`change`,()=>bi(k.simpleNodeSelect.value)),k.simpleConfigSelect.addEventListener(`change`,()=>xi(k.simpleConfigSelect.value)),k.simpleFieldFilter.addEventListener(`input`,()=>Si(k.simpleFieldFilter.value)),k.simpleAddFieldButton.addEventListener(`click`,Ei),k.simpleNewButton.addEventListener(`click`,wi),k.simpleCopyButton.addEventListener(`click`,Ti),k.simpleDeleteButton.addEventListener(`click`,()=>$(()=>Mi(Q))),k.simpleConfigEditor.addEventListener(`change`,e=>Di(e.target)),k.simpleConfigEditor.addEventListener(`toggle`,e=>Ci(e.target),!0),k.simpleConfigEditor.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.fieldValues;if(n){ki(n,`field`);return}let r=t?.dataset.fieldModelValues;if(r){ki(r,`model`);return}let i=t?.dataset.removeSimpleField;i&&Oi(i)}),k.simpleFieldSidebar.addEventListener(`click`,e=>{D(e)?.dataset.closeFieldSidebar!==void 0&&(r.simpleCook.sidebar=null,X())}),k.advancedPreviewButton.addEventListener(`click`,()=>$(Yr)),k.advancedApplyButton.addEventListener(`click`,()=>$(()=>Xr(Q))),k.clearConstructorButton.addEventListener(`click`,Er),k.advancedBackendSelect.addEventListener(`change`,()=>Nr(k.advancedBackendSelect.value)),k.paletteList.addEventListener(`dragstart`,e=>{if(!(e instanceof DragEvent))return;let t=at(e.target,`[data-drag-payload]`,HTMLElement)?.dataset.dragPayload;!t||!e.dataTransfer||(e.dataTransfer.setData(`text/plain`,t),e.dataTransfer.effectAllowed=`copy`)}),k.paletteList.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.addOption;if(n){Tr(n);return}let i=t?.dataset.selectPayload;i&&wr(r.palettePayloads[i])}),k.constructorLanes.addEventListener(`dragover`,e=>{let t=at(e.target,`[data-drop-lane]`,HTMLElement);t&&(e.preventDefault(),t.classList.add(`drag-over`))}),k.constructorLanes.addEventListener(`dragleave`,e=>{at(e.target,`[data-drop-lane]`,HTMLElement)?.classList.remove(`drag-over`)}),k.constructorLanes.addEventListener(`drop`,e=>{if(!(e instanceof DragEvent))return;let t=at(e.target,`[data-drop-lane]`,HTMLElement);!t||!e.dataTransfer||(e.preventDefault(),t.classList.remove(`drag-over`),wr(r.palettePayloads[e.dataTransfer.getData(`text/plain`)],t.dataset.dropLane))}),k.constructorLanes.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.clearLane;if(n){Dr(n);return}let r=t?.dataset.editLaneFields;r&&Or(r)}),k.constructorLanes.addEventListener(`change`,e=>Mr(e.target)),k.constructorFieldDialog.addEventListener(`cancel`,e=>{e.preventDefault(),Gn()}),k.constructorFieldDialog.addEventListener(`click`,e=>{qn(e.target,q)}),k.constructorFieldDialog.addEventListener(`change`,e=>{Kn(e.target)}),k.webuiDialog.addEventListener(`cancel`,e=>{e.preventDefault(),ui()}),k.webuiDialog.addEventListener(`click`,e=>{let t=D(e);if(t?.dataset.webuiDialogClose!==void 0){ui();return}let n=t?.dataset.webuiEnable;if(n){$(()=>si(n,!0));return}let r=t?.dataset.webuiLoad;r&&$(()=>di(r,t.dataset.webuiLoadModel||``,t.dataset.webuiLoadImage||``))}),k.selectedOptionsList.addEventListener(`input`,e=>kr(e.target)),k.selectedOptionsList.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.removeOption;if(n){Ar(n);return}let r=t?.dataset.toggleList;r&&jr(r)}),k.usedModelsList.addEventListener(`click`,e=>{let t=D(e)?.dataset.toggleList;t&&jr(t)}),k.recipesList.addEventListener(`click`,e=>{ha(e)}),ra();async function la(){k.loginError.textContent=``;try{r.csrf=(await ee(k.tokenInput.value)).csrf,Qi(),await ia()}catch(e){k.loginError.textContent=va(e)}}async function ua(){await te(),r.csrf=``,Zi()}async function da(){r.router=await re(),Z(),await J()}async function fa(){r.router=await ie(),Z(),await J()}async function pa(){r.router=await ae(),Z(),await J()}async function ma(){r.router=await oe(),Z(),await J()}async function ha(e){let t=D(e)?.dataset.deleteRecipe;t&&(await _e(t),await Q(),ta())}function $(e){e()}function ga(e){return e===`quick`||e===`constructor`}function _a(e){return e===`configs`||e===`files`||e===`options`}function va(e){return e instanceof Error?e.message:String(e)}