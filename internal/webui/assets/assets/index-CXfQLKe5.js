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
    <option value="${d(M(e))}" ${M(e)===r.benchmark.modelKey?`selected`:``}>
      ${u(xt(e))}
    </option>
  `).join(``),k.benchmarkTypeSelect.value=r.benchmark.type,k.benchmarkAllSections.checked=St(),k.benchmarkSections.innerHTML=l.map(e=>`
    <label class="toggle-row">
      <input type="checkbox" value="${d(e)}" data-benchmark-section="${d(e)}" ${r.benchmark.sections.includes(e)?`checked`:``} ${r.benchmark.type===`general`||St()?`disabled`:``}>
      <span>${u(e)}</span>
    </label>
  `).join(``),k.runBenchmarkButton.disabled=r.benchmark.running||!j(),ft(),pt()}async function ot(){let e=j();if(!e){r.benchmark.record=null,A();return}r.benchmark.error=``,r.benchmark.record=await de(e.node_id||``,e.local_id),A()}async function st(){let e=j();if(e){r.benchmark.running=!0,r.benchmark.error=``,A();try{r.benchmark.record=await fe({node_id:e.node_id||``,model_id:e.local_id,type:r.benchmark.type,sections:r.benchmark.type===`general`||St()?[`all`]:r.benchmark.sections,iterations:1,timeout_seconds:1800})}catch(e){r.benchmark.error=e instanceof Error?e.message:String(e)}finally{r.benchmark.running=!1,A()}}}function ct(e){r.benchmark.modelKey=e,r.benchmark.record=null,A()}function lt(e){r.benchmark.type=e===`section`?`section`:`general`,A()}function ut(e){r.benchmark.sections=e?[...l]:[],A()}function dt(){let e=Array.from(k.benchmarkSections.querySelectorAll(`[data-benchmark-section]`)).filter(e=>e instanceof HTMLInputElement&&e.checked).map(e=>e.value).filter(Ct);r.benchmark.sections=e,A()}function ft(){let e=vt(),t=e?.latest;if(r.benchmark.error){k.benchmarkLatest.innerHTML=`<div class="error-text">${u(r.benchmark.error)}</div>`;return}if(!t){k.benchmarkLatest.innerHTML=`<div class="detail-empty">No benchmark data</div>`;return}let n=l.map(t=>e?.sections?.[t]).filter(e=>!!e);k.benchmarkLatest.innerHTML=[mt(`Latest`,t),...n.map(e=>mt(e.section,e))].join(``)}function pt(){let e=vt()?.history??[];if(e.length===0){k.benchmarkHistory.innerHTML=`<div class="detail-empty">No history yet</div>`;return}k.benchmarkHistory.innerHTML=e.slice().reverse().map(e=>`
    <article class="benchmark-row">
      <div>
        <strong>${u(e.section)} / ${u(e.status)}</strong>
        <div class="muted">${wt(e.finished_at)} / ${e.duration_ms||0}ms</div>
      </div>
      <div class="change-list">${_t(e)}</div>
    </article>
  `).join(``)}function mt(e,t){return`
    <article class="benchmark-card">
      <strong>${u(e)}</strong>
      <div class="benchmark-status ${d(t.status)}">${u(t.status)}</div>
      <div class="muted">${t.duration_ms||0}ms / ${wt(t.finished_at)}</div>
      ${t.error?`<div class="error-text">${u(t.error)}</div>`:``}
      <div class="metric-list">${(t.metrics??[]).map(e=>`
        <span>${u(e.name)}: ${u(ht(e))}</span>
      `).join(``)}</div>
    </article>
  `}function ht(e){return e.duration_ms?`${e.duration_ms}ms`:e.value!==void 0&&e.unit?`${gt(e.value)} ${e.unit}`:e.value===void 0?e.status:gt(e.value)}function gt(e){return Number.isInteger(e)?e.toString():e.toFixed(2)}function _t(e){let t=e.option_changes??[];return t.length===0?`<span class="muted">no option changes</span>`:t.map(e=>`
    <span class="chip amber">${u(e.key)} ${u(e.kind)}</span>
  `).join(``)}function vt(){if(r.benchmark.record)return r.benchmark.record;let e=j();if(!e?.benchmark)return null;let t={node_id:e.node_id,model_id:e.local_id,history:[]};return e.benchmark.latest&&(t.latest=e.benchmark.latest),e.benchmark.sections&&(t.sections=e.benchmark.sections),t}function yt(){r.benchmark.modelKey&&j()||(r.benchmark.modelKey=M(bt()[0]))}function j(){return bt().find(e=>M(e)===r.benchmark.modelKey)??null}function bt(){return Ve()}function M(e){return e?`${e.node_id}\n${e.local_id}`:``}function xt(e){return`${e.node_id||`node`} / ${e.local_id||e.public_id}`}function St(){return r.benchmark.sections.length===l.length}function Ct(e){return l.includes(e)}function wt(e){return e?new Date(e).toLocaleString():`never`}var Tt=[{value:`24h`,label:`Last 24 hours`},{value:`7d`,label:`Last 7 days`},{value:`30d`,label:`Last 30 days`},{value:`90d`,label:`Last 90 days`},{value:`all`,label:`All time`}],Et=[{value:``,label:`All sections`},{value:`llm`,label:`LLM`},{value:`embed`,label:`Embeddings`},{value:`image`,label:`Images`},{value:`voice`,label:`Voice`},{value:`music`,label:`Music`}];function Dt(e){return[{value:``,label:`All nodes`},...Pt((e?.nodes??[]).map(e=>e.node_id).filter(Ft)).map(e=>({value:e,label:e}))]}function Ot(e){return[{value:``,label:`All models`},...Pt((e?.nodes??[]).flatMap(e=>e.models??[]).map(e=>e.local_id||e.public_id).filter(Ft)).map(e=>({value:e,label:e}))]}function kt(e){let t={period:e.period||`24h`};return e.node_id&&(t.node_id=e.node_id),e.model_id&&(t.model_id=e.model_id),e.section&&(t.section=e.section),t}function N(e){return Math.round(Number.isFinite(e??0)?e??0:0).toLocaleString(`en-US`)}function P(e,t=1){let n=Number.isFinite(e??0)?e??0:0;return Number.isInteger(n)?n.toLocaleString(`en-US`):n.toLocaleString(`en-US`,{maximumFractionDigits:t,minimumFractionDigits:n>0&&n<10?t:0})}function At(e){let t=Number.isFinite(e??0)?e??0:0;if(t<60)return`${P(t,1)}s`;let n=t/60;return n<60?`${P(n,1)}m`:`${P(n/60,1)}h`}function jt(e,t,n){let r={points:[],linePath:``,ticks:[]};if(e.length===0||t<=0||n<=0)return r;let i=Math.max(...e.map(e=>e.request_count),1),a=Math.max(0,t-8),o=Math.max(0,n-8),s=e.length-1,c=e.map((e,t)=>({x:4+(s===0?.5:t/s)*a,y:4+(1-e.request_count/i)*o,radius:4}));return{points:c,linePath:c.map((e,t)=>`${t===0?`M`:`L`} ${e.x.toFixed(2)} ${e.y.toFixed(2)}`).join(` `),ticks:Mt(e,c)}}function Mt(e,t){if(e.length===0||t.length===0)return[];let n=e.length-1,r=n<=3?e.map((e,t)=>t):[0,Math.round(n/3),Math.round(n*2/3),n];return Array.from(new Set(r)).map(n=>({x:t[n]?.x??0,label:Nt(e[n]?.bucket_start)}))}function Nt(e){return e?new Date(e).toLocaleDateString(`en-US`,{month:`short`,day:`numeric`}):``}function Pt(e){return Array.from(new Set(e)).sort((e,t)=>e.localeCompare(t))}function Ft(e){return!!e?.trim()}async function F(){r.analytics.loading=!0,r.analytics.error=``,It();try{r.analytics.data=await pe(kt(r.analytics.query))}catch(e){r.analytics.error=e instanceof Error?e.message:String(e)}finally{r.analytics.loading=!1,It()}}function It(){Vt();let e=r.analytics.data;r.analytics.error?k.analyticsStatus.innerHTML=`<div class="error-text">${u(r.analytics.error)}</div>`:r.analytics.loading?k.analyticsStatus.innerHTML=`<div class="detail-empty">Loading analytics</div>`:e?.enabled?k.analyticsStatus.innerHTML=``:k.analyticsStatus.innerHTML=`<div class="detail-empty">Analytics disabled</div>`,Ht(),Ut(),Wt(),Gt()}function Lt(e){tn(e)&&(r.analytics.query.period=e)}function Rt(e){e?r.analytics.query.node_id=e:delete r.analytics.query.node_id}function zt(e){e?r.analytics.query.model_id=e:delete r.analytics.query.model_id}function Bt(e){e?r.analytics.query.section=e:delete r.analytics.query.section}function Vt(){let e=kt(r.analytics.query);k.analyticsPeriodSelect.innerHTML=$t(Tt,e.period),k.analyticsNodeSelect.innerHTML=$t(en(Dt(r.inventory),e.node_id),e.node_id||``),k.analyticsModelSelect.innerHTML=$t(en(Ot(r.inventory),e.model_id),e.model_id||``),k.analyticsSectionSelect.innerHTML=$t(Et,e.section||``)}function Ht(){let e=r.analytics.data?.summary;if(!r.analytics.data?.enabled||!e){k.analyticsSummary.innerHTML=``;return}k.analyticsSummary.innerHTML=[I(`Requests`,N(e.request_count),`${N(e.success_count)} ok / ${N(e.failure_count)} failed`),I(`Tokens`,N(e.total_tokens),`${N(e.input_tokens)} in / ${N(e.output_tokens)} out`),I(`Speed`,`${P(e.average_tokens_per_second,1)} tok/s`,`${P(e.average_duration_ms,0)}ms avg`),I(`Images`,N(e.image_count),`generated or returned`),I(`Audio`,At(e.audio_seconds),`${N(e.audio_tokens)} tokens`)].join(``)}function Ut(){let e=r.analytics.data?.timeline??[];if(!r.analytics.data?.enabled||e.length===0){k.analyticsTimeline.innerHTML=``;return}let t=jt(e,720,170);k.analyticsTimeline.innerHTML=`
    <div class="analytics-chart-head">
      <strong>Timeline</strong>
      <span class="muted">${u(r.analytics.data.granularity)}</span>
    </div>
    <svg class="analytics-chart" viewBox="0 0 720 220" role="img" aria-label="Analytics timeline">
      <path class="analytics-line" d="${d(t.linePath)}"></path>
      ${t.points.map((t,n)=>{let r=e[n];return r?`
        <circle class="analytics-point" cx="${t.x.toFixed(2)}" cy="${t.y.toFixed(2)}" r="${t.radius.toFixed(2)}">
          <title>${u(rn(r))}: ${N(r.request_count)} requests</title>
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
  `}function Wt(){let e=r.analytics.data?.sections??[];if(!r.analytics.data?.enabled||e.length===0){k.analyticsSections.innerHTML=``;return}let t=Math.max(...e.map(e=>e.request_count),1);k.analyticsSections.innerHTML=`
    <div class="analytics-chart-head">
      <strong>Sections</strong>
      <span class="muted">requests by lane</span>
    </div>
    <div class="analytics-section-bars">
      ${e.map(e=>Kt(e,t)).join(``)}
    </div>
  `}function Gt(){let e=r.analytics.data;if(!e?.enabled){k.analyticsModelsTable.innerHTML=``,k.analyticsNodesTable.innerHTML=``,k.analyticsRecentTable.innerHTML=``,k.analyticsNodeErrors.innerHTML=``;return}k.analyticsModelsTable.innerHTML=e.models.map(qt).join(``),k.analyticsNodesTable.innerHTML=e.nodes.map(Jt).join(``),k.analyticsRecentTable.innerHTML=e.recent.map(Yt).join(``),k.analyticsNodeErrors.innerHTML=(e.node_errors??[]).map(e=>`
    <div class="error-text">${u(e.node_id||e.node_url||`node`)}: ${u(e.error)}</div>
  `).join(``)}function I(e,t,n){return`
    <article class="analytics-metric">
      <span>${u(e)}</span>
      <strong>${u(t)}</strong>
      <small>${u(n)}</small>
    </article>
  `}function Kt(e,t){let n=Math.max(1,Math.round(e.request_count/t*100));return`
    <div class="analytics-section-row">
      <span>${u(nn(e.section))}</span>
      <svg viewBox="0 0 100 8" role="img" aria-label="${d(e.section)} requests">
        <rect class="analytics-bar-track" x="0" y="0" width="100" height="8"></rect>
        <rect class="analytics-bar" x="0" y="0" width="${n}" height="8"></rect>
      </svg>
      <strong>${N(e.request_count)}</strong>
    </div>
  `}function qt(e){return`
    <tr>
      <td>${u(e.node_id)}</td>
      <td>${u(e.model_id||`unknown`)}</td>
      <td>${N(e.request_count)}</td>
      <td>${N(e.total_tokens)}</td>
      <td>${N(e.image_count)}</td>
      <td>${At(e.audio_seconds)}</td>
    </tr>
  `}function Jt(e){return`
    <tr>
      <td>${u(e.node_id)}</td>
      <td>${N(e.request_count)}</td>
      <td>${N(e.total_tokens)}</td>
      <td>${N(e.image_count)}</td>
      <td>${At(e.audio_seconds)}</td>
    </tr>
  `}function Yt(e){let t=e.section===`image`?Zt(e):e.section===`voice`||e.section===`music`?Qt(e):Xt(e);return`
    <tr>
      <td>${u(an(e.finished_at))}</td>
      <td>${u(e.node_id)}</td>
      <td>${u(e.model_id||`unknown`)}</td>
      <td>${u(nn(e.section))}</td>
      <td>${u(e.backend_mode||``)}</td>
      <td>${u(e.success?`ok`:String(e.status_code))}</td>
      <td>${u(t)}</td>
    </tr>
  `}function Xt(e){let t=e.tokens_per_second?` / ${P(e.tokens_per_second,1)} tok/s`:``;return`${N(e.input_tokens)} in / ${N(e.output_tokens)} out${t}`}function Zt(e){let t=e.image_width&&e.image_height?` / ${e.image_width}x${e.image_height}`:``;return`${e.image_type?`${e.image_type} / `:``}${N(e.image_count)} images${t}`}function Qt(e){return`${At(e.audio_seconds)} / ${N(e.audio_tokens)} tokens`}function $t(e,t){return e.map(e=>`
    <option value="${d(e.value)}" ${e.value===t?`selected`:``}>${u(e.label)}</option>
  `).join(``)}function en(e,t){return!t||e.some(e=>e.value===t)?e:[...e,{value:t,label:t}]}function tn(e){return e===`24h`||e===`7d`||e===`30d`||e===`90d`||e===`all`}function nn(e){return Et.find(t=>t.value===e)?.label??e}function rn(e){return an(e.bucket_start)}function an(e){return e?new Date(e).toLocaleString():`never`}var L=[`text`,`image`,`embeddings`,`voice`,`music`],R=`backend_mode`,z=[`kobold`,`llama_sdcpp`];[...L],[...L];var on={kobold:`Kobold`,llama_sdcpp:`llama/sd.cpp`},B={text:{label:`LLM`,shortLabel:`Text`,section:`llm`,accent:`cyan`,dropLabel:`Drop a text config or model file`},image:{label:`Image`,shortLabel:`Image`,section:`image`,accent:`magenta`,dropLabel:`Drop an image config or model file`},embeddings:{label:`Embed`,shortLabel:`Embed`,section:`embed`,accent:`lime`,dropLabel:`Drop an embedding config or model file`},voice:{label:`Voice`,shortLabel:`Voice`,section:`voice`,accent:`amber`,dropLabel:`Drop Whisper, TTS, tokenizer, or voice dir`},music:{label:`Music`,shortLabel:`Music`,section:`music`,accent:`violet`,dropLabel:`Drop Music LLM, embeddings, diffusion, or VAE`}};function V(e){return L.includes(e)}var sn={voice:[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],music:[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]};function cn(e){return e===`voice`||e===`music`?sn[e]:[]}function ln(e,t){return(t===`voice`||t===`music`)&&e.component.source===`file`&&!e.component.option_key}function un(e,t){return e!==`voice`&&e!==`music`?!1:sn[e].includes(t)}function dn(e,t){let n={};for(let[r,i]of Object.entries(e))pn(i)!==pn(t[r])&&(n[r]=i);return n}function H(e){return JSON.parse(JSON.stringify(e||{}))}function fn(e){return`${e.backendMode}\n${e.section}\n${e.name}`}function pn(e){return JSON.stringify(e??null)??``}var mn=[`llm`,`image`,`embed`,`voice`,`music`,`runtime`,`other`],hn={llm:`LLM`,image:`Image`,embed:`Embed`,voice:`Voice`,music:`Music`,runtime:`Runtime`,other:`Other`},gn={llm:[`model_param`,`model`],image:[`sdmodel`],embed:[`embeddingsmodel`,`mmproj`],voice:[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],music:[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`]};function U(){return S(r.simpleCook.nodeID)??(r.inventory?.nodes??[])[0]??null}function W(){return(U()?.models??[]).find(e=>e.local_id===r.simpleCook.configID)??null}function _n(){let e=U(),t=e?.node_id||``,n=e?.models??[];return{node:e,nodeFiles:He().filter(e=>e.node_id===t),nodeModels:n,otherNodeModels:n.filter(e=>e.local_id!==r.simpleCook.configID),comparableBySection:new Map}}function vn(e,t){let n=new Map(mn.map(e=>[e,[]]));for(let r of Object.keys(e).sort((e,t)=>e.localeCompare(t))){let e=jn(t(r)),i=n.get(e)??[];i.push(r),n.set(e,i)}return mn.map(e=>({section:e,keys:n.get(e)??[]})).filter(e=>e.keys.length>0)}function yn(e,t,n){return In([...t?.choices??[],...On(t,n),...kn(e,n)].map(e=>Fn(e,t)))}function bn(e,t,n){let i=r.simpleCook.fields[e],a=Mn(r.simpleCook.fields,t),o=An(t,n),s=o.map(t=>t.options?.[e]).filter(e=>!y(e));if(s.length===0)return a&&o.length===0&&!y(i)?`compare-same`:`compare-none`;let c=Re(i);return s.every(e=>Re(e)===c)?`compare-same`:`compare-different`}function xn(e,t,n,r){let i=jn(n(e)),a=t===`model`?An(i,r):r.otherNodeModels,o=[],s=new Set;for(let t of a){let n=t.options?.[e];if(y(n))continue;let r=_(n),i=`${r}\n${t.local_id}`;s.has(i)||(s.add(i),o.push({value:r,config:Tn(t)}))}return o}function Sn(e){let t=e?.hardware,n={quiet:!0,nomodel:!1,contextsize:4096,threads:t?.max_threads?Math.max(1,Math.floor(t.max_threads/2)):-1,batchsize:512,usemmap:!0,usemlock:!1,gpulayers:t?.gpu_backend&&t.gpu_backend!==`cpu`&&t.gpu_backend!==`unknown`?`auto`:`0`};(t?.gpu_backend===`cuda`||t?.gpu_backend===`rocm`)&&(n.usecuda=!0),t?.gpu_backend===`vulkan`&&(n.usevulkan=!0);let r=Ln(e?.node_url||``);return r&&(n.host=r.hostname,r.port&&(n.port=Number(r.port))),n}function Cn(e){if(e?.default!==void 0&&e.default!==``)return v(e,e.default);switch(e?.value_type){case`bool`:return!1;case`number`:return 0;case`json`:return{};default:return``}}function G(e){return JSON.parse(JSON.stringify(e||{}))}function wn(e){return`${e.node_id||`node`} / ${e.backend_mode||`backend`}`}function Tn(e){return`${e.local_id||e.public_id||`config`} / ${e.filename||``}`}function En(e,t){return`${(e?.node_id||`node`).toLowerCase().replace(/[^a-z0-9_-]+/g,`-`).replace(/^-|-$/g,``)||`node`}-${t}`}function Dn(e){return String(e).replace(/[^a-z0-9_-]/gi,`-`)}function On(e,t){if(!e?.model_role)return[];let n=t.nodeFiles.filter(t=>Nn(g(t),e.model_role??``)).map(e=>e.path),r=t.nodeModels.flatMap(t=>Pn(t,e.model_role??``));return[...n,...r]}function kn(e,t){return t.nodeModels.map(t=>t.options?.[e]).filter(e=>!y(e)).map(_)}function An(e,t){let n=t.comparableBySection.get(e);if(n)return n;let i=Mn(r.simpleCook.fields,e),a=t.otherNodeModels;return i?(a=a.filter(t=>Mn(t.options??{},e)===i),t.comparableBySection.set(e,a),a):(t.comparableBySection.set(e,a),a)}function jn(e){return e?.section||`other`}function Mn(e,t){for(let n of gn[t]??[]){let t=e?.[n];if(!y(t))return Re(t)}return``}function Nn(e,t){return t===`llm`?e.includes(`llm`):t===`image`?e.includes(`image`):t===`embeddings`?e.includes(`embeddings`)||e.includes(`llm`):t===`multimodal`?e.includes(`multimodal`):t===`vae`?e.includes(`vae`):t===`clip`?e.includes(`clip`):t===`t5`?e.includes(`t5`):t===`upscaler`?e.includes(`upscaler`):t===`lora`?e.includes(`lora`):t===`voice`?e.includes(`voice`):t===`music`?e.includes(`music`):!0}function Pn(e,t){let n=e.capabilities??{},r=[];return t===`llm`&&typeof e.filename==`string`&&r.push(e.filename),t===`image`&&n.image?.model&&r.push(n.image.model),t===`embeddings`&&n.embeddings?.model&&r.push(n.embeddings.model),t===`multimodal`&&n.multimodal?.projector&&r.push(n.multimodal.projector),t===`vae`&&n.image?.vae&&r.push(n.image.vae),t===`clip`&&r.push(n.image?.clip1,n.image?.clip2,n.image?.clip_l,n.image?.clip_g),t===`t5`&&n.image?.t5xxl&&r.push(n.image.t5xxl),t===`upscaler`&&n.image?.upscaler&&r.push(n.image.upscaler),t===`lora`&&r.push(...n.image?.lora??[]),t===`voice`&&r.push(n.voice?.whisper_model,n.voice?.tts_model,n.voice?.wav_tokenizer,n.voice?.directory),t===`music`&&r.push(n.music?.llm,n.music?.embeddings,n.music?.diffusion,n.music?.vae),r.filter(e=>!!e)}function Fn(e,t){if(t?.value_type===`json`)try{return JSON.parse(e),e}catch{return JSON.stringify(e)}return e}function In(e){let t=new Set,n=[];for(let r of e){let e=String(r??``).trim();!e||t.has(e)||(t.add(e),n.push(e))}return n}function Ln(e){try{return new URL(e)}catch{return null}}var Rn=`tensors-router.constructorFieldPresets`;function zn(){if(!(r.constructor.fieldPresets.length>0))try{let e=JSON.parse(window.localStorage.getItem(Rn)||`[]`);r.constructor.fieldPresets=Array.isArray(e)?e.filter(lr):[]}catch{r.constructor.fieldPresets=[]}}function Bn(e,t){zn(),r.constructor.fieldEditor={lane:e,draft:H(r.constructor.laneOptions[e])},t&&(r.constructor.fieldEditor.pendingPayload=t),K(),cr()}function Vn(){r.constructor.fieldEditor=null,k.constructorFieldDialog.close(),k.constructorFieldDialogBody.innerHTML=``}function K(){let e=r.constructor.fieldEditor;if(!e){k.constructorFieldDialog.open&&Vn();return}let t=e.lane,n=B[t],i=nr(e.pendingPayload??r.constructor.lanes[t]),a=Qn(t,i,e.draft);k.constructorFieldDialogBody.innerHTML=`
    <div class="field-dialog-head">
      <div>
        <h3>${u(n.label)} Fields</h3>
        <p class="muted">${u(n.section)} staged overrides</p>
      </div>
      <button class="icon-button" type="button" title="Close" data-field-modal-action="cancel">x</button>
    </div>
    ${e.pendingPayload?Zn(t,e.pendingPayload):``}
    <div class="preset-row">
      <label>
        Preset
        <select data-field-preset-select>${er(t)}</select>
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
        <select data-field-add-select>${$n(t,a)}</select>
      </label>
      <button type="button" data-field-modal-action="add-field">Add Field</button>
    </div>
    <div class="field-diff-grid">
      ${a.map(t=>Xn(t,i[t],e.draft)).join(``)||`<div class="detail-empty">No fields in this section</div>`}
    </div>
    <div class="field-dialog-actions">
      <button type="button" data-field-modal-action="reset-section">Reset Section</button>
      <span></span>
      <button type="button" data-field-modal-action="cancel">Cancel</button>
      <button type="button" data-field-modal-action="apply">Apply</button>
    </div>
  `}function Hn(e){let t=r.constructor.fieldEditor;if(!t||!(e instanceof HTMLInputElement))return;let n=e.dataset.fieldDraft;if(n)try{t.draft[n]=v(x(n),e.value),e.setCustomValidity(``),K()}catch{e.setCustomValidity(`Invalid JSON`),e.reportValidity()}}function Un(e,t){let n=e instanceof HTMLElement?e.closest(`[data-field-modal-action]`):null;if(!(n instanceof HTMLElement))return;let r=n.dataset.fieldModalAction;if(r===`cancel`){Vn();return}if(r===`apply`){Wn(),t();return}if(r===`reset-section`){Gn();return}if(r===`reset-field`){Kn(n.dataset.fieldKey||``);return}if(r===`add-field`){qn();return}if(r===`apply-preset`){Jn();return}r===`save-preset`&&Yn()}function Wn(){let e=r.constructor.fieldEditor;if(!e)return;if(e.pendingPayload){let t=ir();if(!un(e.lane,t)){k.constructorFieldDialogBody.querySelector(`[data-file-option-key]`)?.setAttribute(`aria-invalid`,`true`);return}r.constructor.lanes[e.lane]=ar(e.pendingPayload,t)}let t=nr(r.constructor.lanes[e.lane]);r.constructor.laneOptions[e.lane]=dn(e.draft,t),Vn()}function Gn(){let e=r.constructor.fieldEditor;e&&(e.draft={},K())}function Kn(e){let t=r.constructor.fieldEditor;t&&(delete t.draft[e],K())}function qn(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-add-select]`);!e||!(t instanceof HTMLSelectElement)||!t.value||(e.draft[t.value]=Cn(x(t.value)),K())}function Jn(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-preset-select]`);if(!e||!(t instanceof HTMLSelectElement)||!t.value)return;let n=tr(e.lane).find(e=>fn(e)===t.value);n&&(Object.assign(e.draft,H(n.values)),K())}function Yn(){let e=r.constructor.fieldEditor,t=k.constructorFieldDialogBody.querySelector(`[data-field-preset-name]`);if(!e||!(t instanceof HTMLInputElement)||!t.value.trim())return;let n={name:t.value.trim(),backendMode:rr(e),section:B[e.lane].section,values:H(e.draft)};r.constructor.fieldPresets=[...r.constructor.fieldPresets.filter(e=>fn(e)!==fn(n)),n],window.localStorage.setItem(Rn,JSON.stringify(r.constructor.fieldPresets)),K()}function Xn(e,t,n){let r=x(e),i=Object.hasOwn(n,e),a=i?n[e]:void 0,o=i&&pn(a)!==pn(t);return`
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
  `}function Zn(e,t){if(!ln(t,e))return``;let n=cn(e);return`
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
  `}function Qn(e,t,n){let r=B[e].section,i=new Set;for(let e of b())(e.section||`other`)===r&&i.add(e.key);for(let a of[...Object.keys(t),...Object.keys(n),...or(e)]){let e=x(a);(!e||(e.section||`other`)===r)&&i.add(a)}return Array.from(i).sort((e,t)=>e.localeCompare(t))}function $n(e,t){let n=new Set(t),r=B[e].section;return b().filter(e=>(e.section||`other`)===r&&!n.has(e.key)).sort(sr).map(e=>`<option value="${d(e.key)}">${u(e.key)}</option>`).join(``)}function er(e){return tr(e).map(e=>`<option value="${d(fn(e))}">${u(e.name)}</option>`).join(``)}function tr(e){let t=r.constructor.fieldEditor,n=B[e].section,i=t?rr(t):``;return r.constructor.fieldPresets.filter(e=>e.section===n&&(!e.backendMode||e.backendMode===i))}function nr(e){return H(e?.model?.options??{})}function rr(e){let t=e.pendingPayload??r.constructor.lanes[e.lane];return t?.model?.backend_mode?t.model.backend_mode:S(t?.component.node_id||``)?.backend_mode||`unknown`}function ir(){let e=k.constructorFieldDialogBody.querySelector(`[data-file-option-key]`);return e instanceof HTMLSelectElement?e.value:``}function ar(e,t){return{...e,component:{...e.component,option_key:t}}}function or(e){return cn(e)}function sr(e,t){return e.key.localeCompare(t.key)}function cr(){k.constructorFieldDialog.open||k.constructorFieldDialog.showModal()}function lr(e){if(!e||typeof e!=`object`)return!1;let t=e;return typeof t.name==`string`&&typeof t.backendMode==`string`&&typeof t.section==`string`&&!!t.values&&typeof t.values==`object`&&!Array.isArray(t.values)}function ur(e,t){let n=dr(e),r={};for(let[e,i]of Object.entries(t)){let t=x(e);(!t||t.section===`runtime`||t.section===n)&&(r[e]=i)}return r}function dr(e){return B[e].section}function fr(){let e=Object.entries(r.constructor.lanes).filter(e=>e[1]!==null),t=e.map(([e,t])=>hr(e,t)),n={};for(let[t,i]of e)Object.assign(n,ur(t,i.model?.options??{})),Object.assign(n,r.constructor.laneOptions[t]??{});return Object.assign(n,r.constructor.options),r.constructor.backendTouched&&(n[R]=r.constructor.backendMode),{id:k.advancedCookIdInput.value.trim(),overwrite:k.overwriteInput.checked,components:t,options:n}}function pr(){let e=[],t=fr();t.id||e.push(m(`warning`,`id_missing`,`Config id is empty.`,`id`)),t.components.length===0&&e.push(m(`warning`,`empty_constructor`,`No lanes selected.`,``));for(let[n,r]of Ge(t.components)){let i=S(n),a=qe(n,r,t.options??{}),o=mr(a,i?.backend_mode||`kobold`);o===`kobold`&&Ne(r,`image`)&&Ne(r,`embeddings`)&&e.push(m(`error`,`kobold_image_embeddings_mix`,`Kobold cannot cook image and embeddings into the same config.`,n));let s=i?.hardware?.max_threads||0;for(let i of Ke(n,r,t.options??{}))s>0&&i.value>s&&e.push(m(`error`,`thread_budget_exceeded`,`${i.key} uses ${i.value} threads on a node with ${s} logical CPUs.`,i.key));if(i?.hardware?.gpu_backend===`rocm`)for(let[t,n]of Object.entries(a))x(t)?.cuda_only&&Fe(n)&&e.push(m(`error`,`cuda_on_rocm`,`${t} is CUDA-only on a ROCm node.`,t));if(!i?.hardware?.gpu_backend||i.hardware.gpu_backend===`unknown`){for(let[t,r]of Object.entries(a))if(Pe(t)&&Fe(r)){e.push(m(`warning`,`gpu_backend_unknown`,`GPU backend could not be inferred.`,n));break}}for(let[t]of Object.entries(a)){let n=x(t);n?.known&&(n.backends?.length??0)>0&&!(n.backends??[]).includes(o)&&e.push(m(`warning`,`unsupported_option`,`${t} is not marked as supported by ${o}.`,t))}}return e}function mr(e,t){let n=e[R];return typeof n==`string`&&z.includes(n)?n:z.includes(t)?t:`kobold`}function hr(e,t){let n=r.constructor.targetNodes[e]||t.component.node_id||``,i=S(n),a={...t.component,node_id:n,node_url:i?.node_url||t.component.node_url||``};if(n&&t.component.node_id&&n!==t.component.node_id){let n=gr(e,t);n.path&&(a.source=`file`,a.file_path=n.path,n.optionKey?a.option_key=n.optionKey:delete a.option_key,delete a.model_id,delete a.image_id)}return a}function gr(e,t){let n=t.model?.options??{};return e===`image`?{path:vr(n.sdmodel)||t.file?.path||``}:e===`embeddings`?{path:vr(n.embeddingsmodel)||t.file?.path||``}:e===`voice`?_r(n,[`whispermodel`,`ttsmodel`,`ttswavtokenizer`,`ttsdir`],t.file?.path):e===`music`?_r(n,[`musicllm`,`musicembeddings`,`musicdiffusion`,`musicvae`],t.file?.path):{path:vr(n.model_param)||yr(n.model)||t.file?.path||``}}function _r(e,t,n){for(let n of t){let t=vr(e[n]);if(t)return{path:t,optionKey:n}}let r=n||``;if(!r)return{path:``};let i=t[0];return i?{path:r,optionKey:i}:{path:r}}function vr(e){return typeof e==`string`?e.trim():``}function yr(e){if(typeof e==`string`)return e.trim();if(Array.isArray(e)){for(let t of e)if(typeof t==`string`&&t.trim())return t.trim()}return``}function q(){Ar(),jr(),Mr(),Nr(),K()}function br(e,t){if(!e)return;if(e.type===`option`){xr(e.key);return}let n=V(t)?t:e.lane;if(n===e.lane){if(ln(e,n)){Bn(n,e);return}r.constructor.lanes[n]=e,q()}}function xr(e){let t=x(e);t&&(Object.hasOwn(r.constructor.options,e)||(r.constructor.options[e]=Hr(t)),q())}function Sr(){r.constructor.lanes=e(),r.constructor.targetNodes=t(),r.constructor.laneOptions=n(),r.constructor.backendMode=`kobold`,r.constructor.backendTouched=!1,r.constructor.options={},r.constructor.fieldEditor=null,q()}function Cr(e){V(e)&&(r.constructor.lanes[e]=null,r.constructor.laneOptions[e]={},q())}function wr(e){!V(e)||!r.constructor.lanes[e]||Bn(e)}function Tr(e){if(!(e instanceof HTMLInputElement))return;let t=e.dataset.optionInput;if(t)try{r.constructor.options[t]=v(x(t),e.value),e.setCustomValidity(``),Pr()}catch{e.setCustomValidity(`Invalid JSON`),e.reportValidity()}}function Er(e){delete r.constructor.options[e],q()}function Dr(e){e===`used`&&(r.constructor.showUsedAll=!r.constructor.showUsedAll),e===`options`&&(r.constructor.showOptionsAll=!r.constructor.showOptionsAll),Nr()}function Or(e){if(!(e instanceof HTMLSelectElement))return;let t=e.dataset.laneTarget;V(t)&&(r.constructor.targetNodes[t]=e.value,q())}function kr(e){z.includes(e)&&(r.constructor.backendMode=e,r.constructor.backendTouched=!0,q())}function Ar(){let e=Wr();k.advancedBackendSelect.innerHTML=z.map(t=>{let n=t===e?` selected`:``;return`<option value="${d(t)}"${n}>${u(on[t])}</option>`}).join(``),k.advancedBackendSelect.classList.toggle(`virtual-backend-select`,!r.constructor.backendTouched)}function jr(){let e=k.constructorFilterInput.value.trim().toLowerCase(),t=Fr().filter(t=>!e||JSON.stringify(t).toLowerCase().includes(e));r.palettePayloads={},k.paletteList.innerHTML=t.map(e=>{let t=`payload-${Object.keys(r.palettePayloads).length}`;r.palettePayloads[t]=e.payload;let n=e.payload.type===`option`?`<button type="button" data-add-option="${d(e.payload.key)}">Add</button>`:`<button type="button" data-select-payload="${d(t)}">Use</button>`;return`
      <article class="palette-item" draggable="true" data-drag-payload="${d(t)}">
        <div class="palette-title">
          <strong>${u(e.title)}</strong>
          ${p(e.badge,e.color)}
        </div>
        <div class="muted">${u(e.subtitle)}</div>
        <div class="palette-meta">${e.meta.map(e=>p(e,``)).join(``)}</div>
        ${n}
      </article>
    `}).join(``)||`<div class="detail-empty">No items</div>`}function Mr(){k.constructorLanes.innerHTML=L.map(Rr).join(``);for(let e of L){let t=document.querySelector(`[data-drop-lane="${e}"]`);if(!(t instanceof HTMLElement))continue;let n=r.constructor.lanes[e];if(!n){t.innerHTML=`<div class="lane-empty">${u(B[e].dropLabel)}</div>`;continue}let i=Object.keys(r.constructor.laneOptions[e]??{}).length;t.innerHTML=`
      <article class="selected-card">
        <strong>${u(n.label)}</strong>
        <div class="muted">${u(n.subtitle)}</div>
        <div class="palette-meta">${n.meta.map(e=>p(e,``)).join(``)}</div>
        ${n.component.option_key?`<div class="muted">Assigned to ${u(n.component.option_key)}</div>`:``}
        <label>
          Target node
          <select data-lane-target="${d(e)}">${Ur(e,n)}</select>
        </label>
        <div class="lane-card-actions">
          <button type="button" data-edit-lane-fields="${d(e)}">Edit fields</button>
          ${i?p(`${i} overrides`,B[e].accent):``}
        </div>
      </article>
    `}}function Nr(){Pr();let e=Ir();k.usedModelsList.innerHTML=Vr(e,r.constructor.showUsedAll,`used`).join(``)||`<div class="detail-empty">No models selected</div>`;let t=Lr();k.selectedOptionsList.innerHTML=Vr(t,r.constructor.showOptionsAll,`options`).join(``)||`<div class="detail-empty">No options selected</div>`}function Pr(){let e=pr();k.validationList.innerHTML=e.length?e.map(Ae).join(``):`<div class="detail-empty">Clean</div>`}function Fr(){return r.activePalette===`files`?Ze():r.activePalette===`options`?Qe():Xe()}function Ir(){let e=[];for(let t of L){let n=r.constructor.lanes[t];if(n){e.push(`
      <div class="used-row">
        ${p(B[t].shortLabel,h(t))}
        <span>${u(n.label)}</span>
      </div>
    `);for(let t of Ye(n))e.push(`<div class="muted">${u(t)}</div>`)}}return e}function Lr(){let e=[],t=Je();for(let[n,i]of Object.entries(t).sort(([e],[t])=>e.localeCompare(t)))if(Object.hasOwn(r.constructor.options,n))e.push(Br(n,r.constructor.options[n]));else if(zr(n)){let t=zr(n);e.push(`
        <div class="option-row">
          ${p(n,``)}
          ${t?p(`${B[t].shortLabel} override`,B[t].accent):``}
          <span class="muted">${u(_(i))}</span>
        </div>
      `)}else e.push(`
        <div class="option-row">
          ${p(n,``)}
          <span class="muted">${u(_(i))}</span>
        </div>
      `);return e}function Rr(e){let t=B[e];return`
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
  `}function zr(e){return L.find(t=>Object.hasOwn(r.constructor.laneOptions[t]??{},e))??null}function Br(e,t){return`
    <div class="option-editor">
      <span>${u(e)}</span>
      <input data-option-input="${d(e)}" value="${d(Le(t))}">
      <button type="button" data-remove-option="${d(e)}">Remove</button>
    </div>
  `}function Vr(e,t,n){return e.length<=9||t?e.length>9?[...e,`<button class="link-button" type="button" data-toggle-list="${n}">Show less</button>`]:e:[...e.slice(0,9),`<button class="link-button" type="button" data-toggle-list="${n}">Show all ${e.length}</button>`]}function Hr(e){switch(e.value_type){case`bool`:return!1;case`number`:return 0;case`json`:return{};default:return``}}function Ur(e,t){let n=r.inventory?.nodes??[],i=r.constructor.targetNodes[e]||t.component.node_id||n[0]?.node_id||``;return r.constructor.targetNodes[e]||(r.constructor.targetNodes[e]=i),n.map(e=>{let t=e.node_id===i?` selected`:``;return`<option value="${d(e.node_id)}"${t}>${u(e.node_id||`node`)}</option>`}).join(``)}function Wr(){if(r.constructor.backendTouched&&z.includes(r.constructor.backendMode))return r.constructor.backendMode;for(let e of L){let t=r.constructor.lanes[e]?.model?.options?.[R];if(typeof t==`string`&&z.includes(t))return t}for(let e of L){let t=r.constructor.lanes[e];if(!t)continue;let n=S(r.constructor.targetNodes[e]||t.component.node_id||``)?.backend_mode||``;if(z.includes(n))return n}let e=r.inventory?.nodes?.[0]?.backend_mode||`kobold`;return z.includes(e)?e:`kobold`}async function Gr(){await qr(he,fr())}async function Kr(e){let t=pr().filter(e=>e.severity===`error`);if(t.length>0){q(),k.cookOutput.textContent=JSON.stringify({validation:t},null,2);return}await qr(ge,fr()),await e()}async function qr(e,t){try{let n=await e(t);k.cookOutput.textContent=JSON.stringify(n,null,2),q()}catch(e){k.cookOutput.textContent=JSON.stringify(xe(e),null,2),q()}}async function Jr(e,t){let n=e.trim();if(n){Yr(`Loading ${n}...`,!1);try{await me({model:n}),Yr(`Loaded ${n}`,!1),await t()}catch(e){Yr(e instanceof Error?e.message:String(e),!0)}}}function Yr(e,t){k.modelsActionStatus.textContent=e,k.modelsActionStatus.classList.toggle(`error-text`,t)}function Xr(e,t){let n=t.trim().toLowerCase();return n?e.filter(e=>[e.name,e.backend,e.backend_mode,e.lane,e.node_id,e.url,...e.compatible_models.map(e=>e.id)].join(` `).toLowerCase().includes(n)):e}function Zr(e){let t=new Map;for(let n of e){let e=n.node_id||`local`;t.set(e,[...t.get(e)??[],n])}return Array.from(t.entries()).sort(([e],[t])=>e.localeCompare(t)).map(([e,t])=>({nodeID:e,entries:[...t].sort((e,t)=>e.name.localeCompare(t.name))}))}function Qr(e){return e.enabled?e.requires_loaded_model&&!e.can_open_without_model&&!e.active?{openable:!1,reason:`needs_model`}:{openable:!0,reason:``}:{openable:!1,reason:`disabled`}}function $r(e){let t=Qr(e);return{title:e.name,message:ti(t.reason),canEnable:!e.enabled,canLoad:e.compatible_models.length>0,models:ei(e)}}function ei(e){return[...e.compatible_models].sort((e,t)=>e.active===t.active?e.id.localeCompare(t.id):e.active?-1:1)}function ti(e){switch(e){case`disabled`:return`Enable this WebUI before opening.`;case`needs_model`:return`Load a compatible model before opening.`;default:return`Ready to open.`}}async function J(){r.webuis.loading=!0,r.webuis.error=``,Y();try{r.webuis.data=await ce()}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{r.webuis.loading=!1,Y()}}function ni(e){r.webuis.filter=e,Y()}async function ri(e,t){r.webuis.action=t?`Enabled`:`Disabled`,r.webuis.error=``,Y();try{r.webuis.data=await le({id:e,enabled:t})}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{Y()}}function ii(e){let t=mi(e);if(t){if(!Qr(t).openable){ui(t);return}hi(t.url)}}function ai(e){let t=mi(e);t&&ui(t)}function oi(){k.webuiDialog.close()}async function si(e,t,n){let i=mi(e);if(i){r.webuis.action=`Loading ${t||n||i.name}...`,r.webuis.error=``,Y();try{let a=await ue({id:e,model_id:t,image_id:n});if(await J(),mi(e)?.enabled&&a.url){oi(),hi(a.url);return}r.webuis.action=`Loaded ${a.model_id||a.image_id||i.name}`}catch(e){r.webuis.error=e instanceof Error?e.message:String(e)}finally{Y()}}}function Y(){let e=Xr(r.webuis.data?.data??[],r.webuis.filter);k.webuiStatus.textContent=fi(e.length),k.webuiStatus.classList.toggle(`error-text`,r.webuis.error!==``),k.webuiGrid.innerHTML=e.length?Zr(e).map(ci).join(``):`<div class="detail-empty">No WebUIs</div>`}function ci(e){return`
    <section class="webui-node-group">
      <div class="webui-node-head">
        <h3>${u(e.nodeID)}</h3>
        <span class="pill">${e.entries.length} WebUIs</span>
      </div>
      <div class="webui-cards">
        ${e.entries.map(li).join(``)}
      </div>
    </section>
  `}function li(e){let t=Qr(e);return`
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
      <div class="webui-model-summary">${u(pi(e))}</div>
      <div class="webui-actions">
        <button type="button" data-webui-open="${d(e.id)}">Open</button>
        <button type="button" data-webui-details="${d(e.id)}">${t.openable?`Models`:`Resolve`}</button>
      </div>
    </article>
  `}function ui(e){let t=$r(e);k.webuiDialogBody.innerHTML=`
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
      ${t.canLoad?t.models.map(t=>di(e,t)).join(``):`<div class="detail-empty">No compatible models</div>`}
    </div>
  `,k.webuiDialog.showModal()}function di(e,t){return`
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
  `}function fi(e){return r.webuis.error?r.webuis.error:r.webuis.loading?`Loading...`:r.webuis.action||`${e} WebUIs`}function pi(e){let t=e.active_image_id||e.active_model_id;return t?`Active: ${t}`:`${e.compatible_models.length} compatible`}function mi(e){return(r.webuis.data?.data??[]).find(t=>t.id===e)}function hi(e){let t=window.open(e,`_blank`,`noopener,noreferrer`);t&&(t.opener=null)}function X(){ki(),Ai(),ji(),Mi(),Ni()}function gi(e){r.simpleCook.nodeID=e,Hi((U()?.models??[])[0]??null),X()}function _i(e){Hi((U()?.models??[]).find(t=>t.local_id===e)??null),X()}function vi(e){r.simpleCook.fieldFilter=e,Mi()}function yi(e){if(!(e instanceof HTMLDetailsElement))return;let t=e.dataset.simpleSection;if(!t)return;let n=new Set(r.simpleCook.openSections);e.open?n.add(t):n.delete(t),r.simpleCook.openSections=Array.from(n)}function bi(){let e=U();r.simpleCook.configID=``,r.simpleCook.mode=`new`,r.simpleCook.fields=Sn(e),r.simpleCook.cleanFields={},r.simpleCook.openSections=[],k.cookIdInput.value=En(e,`new-config`),X()}function xi(){let e=W();r.simpleCook.mode=`copy`,r.simpleCook.configID=``,r.simpleCook.fields=G(r.simpleCook.fields),r.simpleCook.cleanFields={},r.simpleCook.openSections=[],k.cookIdInput.value=En(U(),`${e?.local_id||`config`}-copy`),X()}function Si(){let e=k.simpleAddFieldSelect.value;if(!e||Object.hasOwn(r.simpleCook.fields,e))return;let t=x(e);r.simpleCook.fields[e]=Cn(t),X()}function Ci(e){if(!(e instanceof HTMLInputElement)&&!(e instanceof HTMLSelectElement))return;if(e instanceof HTMLSelectElement&&e.dataset.simpleBackendMode!==void 0){r.simpleCook.fields[R]=e.value,X();return}let t=e.dataset.simpleField;t&&(r.simpleCook.fields[t]=v(x(t),e.value),X())}function wi(e){delete r.simpleCook.fields[e],r.simpleCook.sidebar?.key===e&&(r.simpleCook.sidebar=null),X()}function Ti(e,t){r.simpleCook.sidebar={key:e,type:t},Ni()}async function Ei(){await Li(ve)}async function Di(e){let t=await Li(ye);t&&(r.simpleCook.mode=`edit`,r.simpleCook.configID=t.id||``,r.simpleCook.fields=G(t.options??r.simpleCook.fields),r.simpleCook.cleanFields=G(r.simpleCook.fields),await e())}async function Oi(e){let t=W();if(t&&window.confirm(`Delete ${t.filename||t.local_id}?`))try{await be({node_id:r.simpleCook.nodeID,node_url:U()?.node_url||``,id:t.local_id,filename:t.filename,overwrite:!1,options:{}}),await e()}catch(e){Wi(e)}}function ki(){let e=r.inventory?.nodes??[];if(e.length===0){r.simpleCook.nodeID=``,r.simpleCook.configID=``,r.simpleCook.fields={},r.simpleCook.cleanFields={},r.simpleCook.openSections=[];return}if(!e.some(e=>e.node_id===r.simpleCook.nodeID)){let t=e[0];r.simpleCook.nodeID=t?.node_id??``,Hi((t?.models??[])[0]??null);return}if(r.simpleCook.mode!==`edit`)return;let t=W(),n=U();!t&&(n?.models??[]).length>0&&Hi((n?.models??[])[0]??null)}function Ai(){let e=r.inventory?.nodes??[];Ui(k.simpleNodeSelect,e.map(e=>it(e.node_id,wn(e)))),k.simpleNodeSelect.value=r.simpleCook.nodeID;let t=U()?.models??[];Ui(k.simpleConfigSelect,t.map(e=>it(e.local_id,Tn(e)))),k.simpleConfigSelect.value=r.simpleCook.configID,k.simpleConfigSelect.disabled=t.length===0,k.simpleCopyButton.disabled=Object.keys(r.simpleCook.fields||{}).length===0,k.simpleDeleteButton.disabled=!W(),k.simpleFieldFilter.value=r.simpleCook.fieldFilter}function ji(){let e=r.simpleCook.fields||{},t=b().filter(t=>t.key!==`backend_mode`&&!Object.hasOwn(e,t.key)).sort((e,t)=>`${Gi(e)}:${e.key}`.localeCompare(`${Gi(t)}:${t.key}`));k.simpleAddFieldSelect.innerHTML=t.map(e=>{let t=`${hn[Gi(e)]||`Other`} / ${e.key}`;return`<option value="${d(e.key)}">${u(t)}</option>`}).join(``)}function Mi(){let e=r.simpleCook.fields||{},t=r.simpleCook.fieldFilter.trim().toLowerCase(),n=_n(),i=new Set(r.simpleCook.openSections),a=zi(e).map(r=>{let a=r.keys.filter(e=>!t||`${e} ${_(Bi(e))}`.toLowerCase().includes(t)),o=a.map(t=>Pi(t,Bi(t),r.section,n,t===`backend_mode`&&!Object.hasOwn(e,`backend_mode`))).join(``);if(!o)return null;let s=hn[r.section]||r.section;return{section:r.section,html:`
        <details class="config-section" data-simple-section="${d(r.section)}"${i.has(r.section)?` open`:``}>
          <summary>
            <span>${u(s)}</span>
            <span class="section-count">${u(Ki(a.length))}</span>
          </summary>
          <div class="config-fields">${o}</div>
        </details>
      `}}).filter(e=>e!==null);k.simpleConfigEditor.innerHTML=a.length?a.map(e=>e.html).join(``):`<div class="detail-empty">No fields</div>`}function Ni(){let e=r.simpleCook.sidebar;if(!e){k.simpleFieldSidebar.innerHTML=`<div class="detail-empty">Field values</div>`;return}let t=xn(e.key,e.type,x,_n());k.simpleFieldSidebar.innerHTML=`
    <div class="field-sidebar-head">
      <div>
        <h3>${u(e.key)}</h3>
        <p class="muted">${u(e.type===`model`?`same model file`:`same field`)}</p>
      </div>
      <button type="button" data-close-field-sidebar>x</button>
    </div>
    <div class="detail-list">
      ${t.length?t.map(Ii).join(``):`<div class="detail-empty">No values</div>`}
    </div>
  `}function Pi(e,t,n,r,i=!1){let a=x(e),o=`field-values-${Dn(e)}`,s=yn(e,a,r),c=bn(e,n,r),ee=e===`backend_mode`?Fi(Vi(),i):`
        <input data-simple-field="${d(e)}" list="${d(o)}" value="${d(Le(t))}">
        <datalist id="${d(o)}">
          ${s.map(e=>`<option value="${d(e)}"></option>`).join(``)}
        </datalist>
      `,te=gn[n]?`<button class="icon-button" type="button" title="Same model values" data-field-model-values="${d(e)}">M</button>`:``;return`
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
  `}function Fi(e,t){let n=z.includes(e)?e:`kobold`;return`
    <select data-simple-backend-mode class="${t?`virtual-backend-select`:``}">
      ${z.map(e=>`<option value="${d(e)}"${e===n?` selected`:``}>${u(on[e])}</option>`).join(``)}
    </select>
  `}function Ii(e){return`
    <div class="sidebar-value">
      <strong>${u(e.value)}</strong>
      <span class="muted">${u(e.config)}</span>
    </div>
  `}async function Li(e){try{let t=await e(Ri());return k.cookOutput.textContent=JSON.stringify(t,null,2),t}catch(e){return Wi(e),null}}function Ri(){let e=W(),t=k.cookIdInput.value.trim(),n=!!(e&&t===e.local_id);return{node_id:r.simpleCook.nodeID,node_url:U()?.node_url||``,id:t,filename:e?.filename||``,overwrite:n||k.overwriteInput.checked,options:G(r.simpleCook.fields)}}function zi(e){let t=vn(e,x);if(Object.hasOwn(e,`backend_mode`))return t;let n=t.find(e=>e.section===`runtime`);return n?(n.keys=[R,...n.keys],t):[...t,{section:`runtime`,keys:[R]}]}function Bi(e){return e===`backend_mode`&&!Object.hasOwn(r.simpleCook.fields,`backend_mode`)?Vi():r.simpleCook.fields[e]}function Vi(){let e=r.simpleCook.fields[R];if(typeof e==`string`&&z.includes(e))return e;let t=W()?.backend_mode||U()?.backend_mode||`kobold`;return z.includes(t)?t:`kobold`}function Hi(e){r.simpleCook.mode=`edit`,r.simpleCook.configID=e?.local_id||``,r.simpleCook.fields=G(e?.options??{}),r.simpleCook.cleanFields=G(e?.options??{}),r.simpleCook.sidebar=null,r.simpleCook.openSections=[],k.cookIdInput.value=e?.local_id||En(U(),`new-config`)}function Ui(e,t){let n=e.value;e.innerHTML=t.map(e=>`<option value="${d(e.value)}">${u(e.label)}</option>`).join(``),Array.from(e.options).some(e=>e.value===n)&&(e.value=n)}function Wi(e){k.cookOutput.textContent=JSON.stringify(xe(e),null,2)}function Gi(e){return e.section||`other`}function Ki(e){return e===1?`1 field`:`${e} fields`}function qi(){k.loginView.classList.remove(`hidden`),k.appView.classList.add(`hidden`)}function Ji(){k.loginView.classList.add(`hidden`),k.appView.classList.remove(`hidden`)}function Yi(){Qi(),Xi(),A(),It(),X(),q(),Zi()}function Z(){let e=r.router;k.routerSummary.textContent=`${e?.url||``} ${e?.running?`running`:`stopped`}`,k.launchButton.disabled=!e?.managed||!!e?.running,k.restartButton.disabled=!e?.managed,k.shutdownButton.disabled=!e?.can_shutdown,k.forceKillButton.disabled=!e?.can_force_kill,k.routerStatus.innerHTML=[f(`Managed`,e?.managed?`yes`:`no`),f(`Running`,e?.running?`yes`:`no`),f(`URL`,e?.url||`unknown`),f(`PID`,e?.pid?String(e.pid):`none`),f(`Can shutdown`,e?.can_shutdown?`yes`:`no`),f(`Can force kill`,e?.can_force_kill?`yes`:`no`),f(`Last error`,e?.error||`none`)].join(``)}function Xi(){let e=k.filterInput.value.trim().toLowerCase(),t=Ue(e),n=We(e);k.modelsTable.innerHTML=t.map(e=>`
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
  `).join(``)}function Zi(){let e=r.inventory?.recipes??[];k.recipeCount.textContent=`${e.length} recipes`,k.recipesList.innerHTML=e.map(e=>`
    <article class="recipe-item">
      <div>
        <strong>${u(e.public_id||e.id)}</strong>
        <div class="muted">${u(e.public_image_id||``)}</div>
      </div>
      <button type="button" data-delete-recipe="${d(e.id)}">Delete</button>
    </article>
  `).join(``)}function Qi(){let e=r.inventory?.nodes??[];k.nodeCount.textContent=`${e.length} nodes`,k.nodesGrid.innerHTML=e.map(e=>{let t=e.hardware;return`
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
    `}).join(``)}async function $i(){try{r.csrf=(await c()).csrf,Ji(),await ea()}catch{qi()}}async function ea(){await ta(),await Q(),await J(),await F()}async function ta(){r.router=await ne(),Z()}async function Q(){r.inventory=await se(),Yi()}function na(e){r.activeTab=e,O(`[data-tab]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.tab===e)),O(`[data-panel]`,HTMLElement).forEach(t=>t.classList.toggle(`active`,t.dataset.panel===e))}function ra(e){fa(e)&&(r.activeCookMode=e,O(`[data-cook-mode]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.cookMode===e)),O(`[data-cook-panel]`,HTMLElement).forEach(t=>t.classList.toggle(`active`,t.dataset.cookPanel===e)))}function ia(e){pa(e)&&(r.activePalette=e,O(`[data-palette]`,HTMLButtonElement).forEach(t=>t.classList.toggle(`active`,t.dataset.palette===e)),q())}O(`[data-tab]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>na(e.dataset.tab||``))}),O(`[data-cook-mode]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>ra(e.dataset.cookMode))}),O(`[data-palette]`,HTMLButtonElement).forEach(e=>{e.addEventListener(`click`,()=>ia(e.dataset.palette))}),k.loginForm.addEventListener(`submit`,e=>{e.preventDefault(),aa()}),k.logoutButton.addEventListener(`click`,()=>$(oa)),k.refreshButton.addEventListener(`click`,()=>$(ea)),k.webuiFilterInput.addEventListener(`input`,()=>ni(k.webuiFilterInput.value)),k.webuiGrid.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.webuiOpen;if(n){ii(n);return}let r=t?.dataset.webuiDetails;r&&ai(r)}),k.webuiGrid.addEventListener(`change`,e=>{let t=D(e),n=t?.dataset.webuiToggle;n&&t instanceof HTMLInputElement&&$(()=>ri(n,t.checked))}),k.filterInput.addEventListener(`input`,Xi),k.modelsTable.addEventListener(`click`,e=>{let t=D(e)?.dataset.loadConfig;t&&$(()=>Jr(t,Q))}),k.benchmarkModelSelect.addEventListener(`change`,()=>{ct(k.benchmarkModelSelect.value),$(ot)}),k.benchmarkTypeSelect.addEventListener(`change`,()=>lt(k.benchmarkTypeSelect.value)),k.benchmarkAllSections.addEventListener(`change`,()=>ut(k.benchmarkAllSections.checked)),k.benchmarkSections.addEventListener(`change`,dt),k.runBenchmarkButton.addEventListener(`click`,()=>$(async()=>{await st(),await Q()})),k.analyticsPeriodSelect.addEventListener(`change`,()=>$(async()=>{Lt(k.analyticsPeriodSelect.value),await F()})),k.analyticsNodeSelect.addEventListener(`change`,()=>$(async()=>{Rt(k.analyticsNodeSelect.value),await F()})),k.analyticsModelSelect.addEventListener(`change`,()=>$(async()=>{zt(k.analyticsModelSelect.value),await F()})),k.analyticsSectionSelect.addEventListener(`change`,()=>$(async()=>{Bt(k.analyticsSectionSelect.value),await F()})),k.analyticsRefreshButton.addEventListener(`click`,()=>$(F)),k.constructorFilterInput.addEventListener(`input`,q),k.launchButton.addEventListener(`click`,()=>$(sa)),k.restartButton.addEventListener(`click`,()=>$(ca)),k.shutdownButton.addEventListener(`click`,()=>$(la)),k.forceKillButton.addEventListener(`click`,()=>$(ua)),k.previewButton.addEventListener(`click`,()=>$(Ei)),k.cookForm.addEventListener(`submit`,e=>{e.preventDefault(),Di(Q)}),k.simpleNodeSelect.addEventListener(`change`,()=>gi(k.simpleNodeSelect.value)),k.simpleConfigSelect.addEventListener(`change`,()=>_i(k.simpleConfigSelect.value)),k.simpleFieldFilter.addEventListener(`input`,()=>vi(k.simpleFieldFilter.value)),k.simpleAddFieldButton.addEventListener(`click`,Si),k.simpleNewButton.addEventListener(`click`,bi),k.simpleCopyButton.addEventListener(`click`,xi),k.simpleDeleteButton.addEventListener(`click`,()=>$(()=>Oi(Q))),k.simpleConfigEditor.addEventListener(`change`,e=>Ci(e.target)),k.simpleConfigEditor.addEventListener(`toggle`,e=>yi(e.target),!0),k.simpleConfigEditor.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.fieldValues;if(n){Ti(n,`field`);return}let r=t?.dataset.fieldModelValues;if(r){Ti(r,`model`);return}let i=t?.dataset.removeSimpleField;i&&wi(i)}),k.simpleFieldSidebar.addEventListener(`click`,e=>{D(e)?.dataset.closeFieldSidebar!==void 0&&(r.simpleCook.sidebar=null,X())}),k.advancedPreviewButton.addEventListener(`click`,()=>$(Gr)),k.advancedApplyButton.addEventListener(`click`,()=>$(()=>Kr(Q))),k.clearConstructorButton.addEventListener(`click`,Sr),k.advancedBackendSelect.addEventListener(`change`,()=>kr(k.advancedBackendSelect.value)),k.paletteList.addEventListener(`dragstart`,e=>{if(!(e instanceof DragEvent))return;let t=at(e.target,`[data-drag-payload]`,HTMLElement)?.dataset.dragPayload;!t||!e.dataTransfer||(e.dataTransfer.setData(`text/plain`,t),e.dataTransfer.effectAllowed=`copy`)}),k.paletteList.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.addOption;if(n){xr(n);return}let i=t?.dataset.selectPayload;i&&br(r.palettePayloads[i])}),k.constructorLanes.addEventListener(`dragover`,e=>{let t=at(e.target,`[data-drop-lane]`,HTMLElement);t&&(e.preventDefault(),t.classList.add(`drag-over`))}),k.constructorLanes.addEventListener(`dragleave`,e=>{at(e.target,`[data-drop-lane]`,HTMLElement)?.classList.remove(`drag-over`)}),k.constructorLanes.addEventListener(`drop`,e=>{if(!(e instanceof DragEvent))return;let t=at(e.target,`[data-drop-lane]`,HTMLElement);!t||!e.dataTransfer||(e.preventDefault(),t.classList.remove(`drag-over`),br(r.palettePayloads[e.dataTransfer.getData(`text/plain`)],t.dataset.dropLane))}),k.constructorLanes.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.clearLane;if(n){Cr(n);return}let r=t?.dataset.editLaneFields;r&&wr(r)}),k.constructorLanes.addEventListener(`change`,e=>Or(e.target)),k.constructorFieldDialog.addEventListener(`cancel`,e=>{e.preventDefault(),Vn()}),k.constructorFieldDialog.addEventListener(`click`,e=>{Un(e.target,q)}),k.constructorFieldDialog.addEventListener(`change`,e=>{Hn(e.target)}),k.webuiDialog.addEventListener(`cancel`,e=>{e.preventDefault(),oi()}),k.webuiDialog.addEventListener(`click`,e=>{let t=D(e);if(t?.dataset.webuiDialogClose!==void 0){oi();return}let n=t?.dataset.webuiEnable;if(n){$(()=>ri(n,!0));return}let r=t?.dataset.webuiLoad;r&&$(()=>si(r,t.dataset.webuiLoadModel||``,t.dataset.webuiLoadImage||``))}),k.selectedOptionsList.addEventListener(`input`,e=>Tr(e.target)),k.selectedOptionsList.addEventListener(`click`,e=>{let t=D(e),n=t?.dataset.removeOption;if(n){Er(n);return}let r=t?.dataset.toggleList;r&&Dr(r)}),k.usedModelsList.addEventListener(`click`,e=>{let t=D(e)?.dataset.toggleList;t&&Dr(t)}),k.recipesList.addEventListener(`click`,e=>{da(e)}),$i();async function aa(){k.loginError.textContent=``;try{r.csrf=(await ee(k.tokenInput.value)).csrf,Ji(),await ea()}catch(e){k.loginError.textContent=ma(e)}}async function oa(){await te(),r.csrf=``,qi()}async function sa(){r.router=await re(),Z(),await J()}async function ca(){r.router=await ie(),Z(),await J()}async function la(){r.router=await ae(),Z(),await J()}async function ua(){r.router=await oe(),Z(),await J()}async function da(e){let t=D(e)?.dataset.deleteRecipe;t&&(await _e(t),await Q(),Zi())}function $(e){e()}function fa(e){return e===`quick`||e===`constructor`}function pa(e){return e===`configs`||e===`files`||e===`options`}function ma(e){return e instanceof Error?e.message:String(e)}