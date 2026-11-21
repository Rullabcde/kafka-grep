package main

// The page targets a Vercel-style monochrome shell: black ground, hairline
// borders, Geist type, one blue accent reserved for focus and the matched field.
const indexHTML = `<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>kafkagrep</title>
<style>
@font-face{font-family:Geist;src:url("/fonts/geist-400.woff2") format("woff2");font-weight:400;font-display:swap}
@font-face{font-family:Geist;src:url("/fonts/geist-500.woff2") format("woff2");font-weight:500;font-display:swap}
@font-face{font-family:Geist;src:url("/fonts/geist-600.woff2") format("woff2");font-weight:600;font-display:swap}
@font-face{font-family:"Geist Mono";src:url("/fonts/geist-mono-400.woff2") format("woff2");font-weight:400;font-display:swap}
@font-face{font-family:"Geist Mono";src:url("/fonts/geist-mono-500.woff2") format("woff2");font-weight:500;font-display:swap}
:root{
  --bg:#000000; --panel:#0A0A0A; --raise:#111111; --hover:#161616;
  --line:#1F1F1F; --line-strong:#2E2E2E; --line-hover:#3F3F3F;
  --fg:#EDEDED; --fg-hover:#CFCFCF; --muted:#A1A1A1; --dim:#7D7D7D;
  --accent:#3291FF; --accent-soft:rgba(50,145,255,0.16);
  --danger:#FF6369;
  --ui:Geist,-apple-system,BlinkMacSystemFont,"Segoe UI",Roboto,sans-serif;
  --mono:"Geist Mono",ui-monospace,SFMono-Regular,Menlo,monospace;
}
*,*::before,*::after{box-sizing:border-box}
html{color-scheme:dark;font-size:16px}
body{margin:0;background:var(--bg);color:var(--fg);font-family:var(--ui);font-size:14px;line-height:20px;letter-spacing:-0.006em;-webkit-font-smoothing:antialiased}
button{cursor:pointer;font:inherit;letter-spacing:inherit}
button:disabled{cursor:not-allowed;opacity:.45}
a{color:var(--accent);text-decoration:none}
:focus-visible{outline:1px solid var(--accent);outline-offset:2px}
::selection{background:var(--accent);color:#000}
.mono{font-family:var(--mono);font-variant-numeric:tabular-nums;letter-spacing:0}

header{position:sticky;top:0;z-index:5;height:48px;display:flex;align-items:center;justify-content:space-between;gap:12px;padding:0 24px;background:rgba(0,0,0,.8);backdrop-filter:blur(8px);border-bottom:1px solid var(--line)}
.brand{display:flex;align-items:center;gap:8px;font-size:14px;font-weight:600}
.brand svg{display:block;color:var(--fg)}
.host{font-family:var(--mono);font-size:12px;line-height:16px;color:var(--dim)}
main{max-width:1100px;margin:0 auto;padding:28px 24px 64px}
.hint{margin:0 0 20px;color:var(--dim);font-size:12px;line-height:16px}
.hint kbd{font-family:var(--mono);font-size:11px;line-height:1;padding:3px 5px;border:1px solid var(--line-strong);border-radius:4px;background:var(--panel);color:var(--muted)}
.toolbar{display:flex;gap:8px;align-items:stretch;margin-bottom:10px}
.querybar{flex:1;display:flex;background:var(--panel);border:1px solid var(--line);border-radius:6px;transition:border-color 120ms ease-out}
.querybar:focus-within{border-color:var(--line-strong)}
.seg{display:flex;flex-direction:column;justify-content:center;gap:1px;padding:7px 12px;min-height:44px;border-left:1px solid var(--line);cursor:text;transition:box-shadow 120ms ease-out}
.seg:first-child{border-left:0;border-radius:6px 0 0 6px}
.seg:last-child{border-radius:0 6px 6px 0}
.seg:focus-within{box-shadow:inset 0 0 0 1px var(--accent);border-radius:4px}
.seg span{font-size:11px;line-height:14px;color:var(--dim);letter-spacing:.01em}
.seg input,.seg select{background:none;border:0;color:var(--fg);font:inherit;font-size:13px;line-height:18px;padding:0;width:100%;outline:none}
.seg input::placeholder{color:var(--dim)}
.seg input.mono,.seg select.mono{font-family:var(--mono);letter-spacing:0}
.seg-topic{flex:0 0 22%}
.seg-filter{flex:1 1 auto;min-width:0}
.seg-start{flex:0 0 108px}
.seg-since{flex:0 0 92px}
.seg-limit{flex:0 0 84px}
select{appearance:none;background-color:transparent;cursor:pointer}
.btn{display:inline-flex;align-items:center;justify-content:center;gap:6px;border-radius:6px;border:1px solid var(--line-strong);background:var(--panel);color:var(--fg);padding:0 14px;min-height:44px;font-size:13px;font-weight:500;transition:background-color 120ms ease-out,border-color 120ms ease-out,color 120ms ease-out}
.btn:hover{background:var(--hover);border-color:var(--line-hover)}
.btn:active{background:var(--raise)}
.btn.primary{background:var(--fg);border-color:var(--fg);color:#000}
.btn.primary:hover{background:var(--fg-hover);border-color:var(--fg-hover)}
.btn.stop{background:var(--panel);border-color:var(--line-strong);color:var(--fg)}
.btn.small{min-height:32px;padding:0 10px;font-size:12px;border-radius:4px}
.status{display:flex;justify-content:space-between;align-items:center;gap:12px;min-height:28px;font-size:12px;line-height:16px;color:var(--muted)}
.status .path{font-family:var(--mono);color:var(--dim)}
.link{border:0;background:none;color:var(--muted);font-size:12px;line-height:16px;padding:8px 0;min-height:32px;transition:color 120ms ease-out}
.link:hover{color:var(--fg)}
.strip{display:flex;gap:2px;height:2px;margin:8px 0 18px;background:var(--line);overflow:hidden;border-radius:2px}
.strip[hidden]{display:none}
.strip-seg{flex:1;background:transparent}
.strip-seg i{display:block;height:100%;width:0;background:var(--fg);transition:width 120ms linear}
.strip-ind{flex:1;overflow:hidden;position:relative;background:var(--line)}
.strip-ind b{display:block;height:100%;width:22%;background:var(--fg);animation:sweep 1.1s ease-in-out infinite}
@keyframes sweep{from{transform:translateX(-100%)}to{transform:translateX(460%)}}
.results{list-style:none;margin:0;padding:0;border-top:1px solid var(--line)}
.results li{border-bottom:1px solid var(--line)}
.row{display:grid;grid-template-columns:108px 150px minmax(0,170px) 1fr;gap:14px;width:100%;padding:11px 8px;background:none;border:0;color:var(--fg);text-align:left;font-size:13px;line-height:20px;transition:background-color 120ms ease-out}
.row:hover{background:var(--panel)}
.row:focus-visible{outline:1px solid var(--accent);outline-offset:-1px}
.row .po,.row .tm{font-family:var(--mono);font-size:12px;color:var(--dim);letter-spacing:0}
.row .po b{color:var(--muted);font-weight:500}
.row .ky{font-family:var(--mono);font-size:12px;color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap;letter-spacing:0}
.row .pv{color:var(--muted);overflow:hidden;text-overflow:ellipsis;white-space:nowrap}
.results li.open .row{background:var(--panel)}
.body{position:relative;background:var(--panel);border-top:1px solid var(--line);padding:14px 14px 16px}
.body[hidden]{display:none}
.body .copy{position:absolute;top:10px;right:10px;width:auto}
.json{font-family:var(--mono);font-size:12px;line-height:20px;margin:0;padding-right:70px;max-height:520px;overflow:auto;white-space:pre;letter-spacing:0;color:var(--muted)}
.json .k{color:var(--dim)}
.json .s{color:var(--fg)}
.json .n{color:#C7C7C7}
.json .l{color:var(--dim)}
.json .match{color:var(--fg);background:var(--accent-soft);border-radius:3px;box-shadow:0 0 0 2px var(--accent-soft)}
.notice{max-width:640px;padding:28px 0}
.notice h2{font-size:20px;line-height:28px;font-weight:600;letter-spacing:-0.02em;margin:0 0 8px}
.notice h2.error{color:var(--danger)}
.notice p{margin:0 0 6px;color:var(--muted);font-size:13px;line-height:20px}
.notice code{font-family:var(--mono);font-size:12px;color:var(--fg);background:var(--raise);border:1px solid var(--line);border-radius:4px;padding:2px 5px;letter-spacing:0}
.notice details{margin-top:14px}
.notice summary{cursor:pointer;font-size:12px;color:var(--dim)}
.notice summary:hover{color:var(--muted)}
.notice pre{font-family:var(--mono);font-size:12px;line-height:18px;color:var(--muted);white-space:pre-wrap;word-break:break-word;margin:8px 0 0;padding:10px;background:var(--raise);border:1px solid var(--line);border-radius:6px;letter-spacing:0}
@media (max-width:800px){
  .toolbar{flex-direction:column}
  .querybar{flex-wrap:wrap}
  .seg{border-left:0;border-top:1px solid var(--line);border-radius:0}
  .seg:first-child{border-top:0;border-radius:6px 6px 0 0}
  .seg-topic,.seg-filter{flex:1 1 100%}
  .seg-start,.seg-since,.seg-limit{flex:1 1 33.33%}
  .seg-limit{border-radius:0 0 6px 0}
  .seg-start{border-radius:0 0 0 6px}
  .toolbar .btn{width:100%}
  .row{grid-template-columns:1fr auto;gap:2px 12px}
  .row .po{grid-column:1}
  .row .tm{grid-column:2;text-align:right}
  .row .ky,.row .pv{grid-column:1 / -1}
  .json{padding-right:0;padding-top:34px}
}
@media (max-width:640px){
  header{padding:0 16px}
  main{padding:20px 16px 48px}
}
@media (prefers-reduced-motion:reduce){
  *{transition:none!important;animation:none!important}
  .strip-ind b{width:40%;transform:none}
}
</style>
</head><body>
<header>
  <span class="brand">
    <svg width="16" height="16" viewBox="0 0 16 16" fill="none" aria-hidden="true"><rect x="1.5" y="2.5" width="13" height="3" rx="1" stroke="currentColor" stroke-width="1.2"/><rect x="1.5" y="7" width="13" height="1.6" rx=".8" fill="currentColor"/><rect x="1.5" y="10.4" width="8" height="1.6" rx=".8" fill="currentColor" opacity=".45"/></svg>
    kafkagrep
  </span>
  <span class="host" id="host"></span>
</header>
<main>
  <p class="hint">Enter a topic and a filter. <kbd>Enter</kbd> to search, <kbd>Esc</kbd> to stop, <kbd>/</kbd> to focus the filter.</p>
  <form id="querybar" class="toolbar" autocomplete="off">
    <div class="querybar">
      <label class="seg seg-topic">
        <span>Topic</span>
        <input id="topic" class="mono" type="text" spellcheck="false" placeholder="news-enriched">
      </label>
      <label class="seg seg-filter">
        <span>Filter</span>
        <input id="filter" class="mono" type="text" spellcheck="false" placeholder='value.id == "..."'>
      </label>
      <label class="seg seg-start">
        <span>Start</span>
        <select id="start" class="mono">
          <option value="from-end">from end</option>
          <option value="from-start">from start</option>
          <option value="from-offset">from offset</option>
        </select>
      </label>
      <label class="seg seg-since">
        <span>Since</span>
        <input id="since" class="mono" type="text" spellcheck="false" placeholder="1d">
      </label>
      <label class="seg seg-limit">
        <span>Limit</span>
        <input id="limit" class="mono" type="number" min="1" value="100">
      </label>
    </div>
    <button id="go" class="btn primary" type="submit">Search</button>
  </form>
  <div class="status">
    <span id="statusText" role="status" aria-live="polite"></span>
    <button id="copyAll" class="link" type="button" hidden>Copy all</button>
  </div>
  <div id="strip" class="strip" hidden></div>
  <ul id="results" class="results"></ul>
  <div id="notice" class="notice" hidden></div>
</main>
<script>
(function(){
  "use strict";
  var d=document;
  function $(s){return d.querySelector(s);}
  var form=$("#querybar"),topicEl=$("#topic"),filterEl=$("#filter"),startEl=$("#start"),
      sinceEl=$("#since"),limitEl=$("#limit"),goBtn=$("#go"),statusText=$("#statusText"),copyAllBtn=$("#copyAll"),
      stripEl=$("#strip"),resultsEl=$("#results"),noticeEl=$("#notice");
  var es=null,running=false,matches=[],scanned=0;

  $("#host").textContent=location.host;

  function esc(s){return String(s).replace(/[&<>"']/g,function(c){
    return ({"&":"&amp;","<":"&lt;",">":"&gt;","\"":"&quot;","'":"&#39;"})[c];});}
  function pad(n){return String(n).padStart(2,"0");}
  function fmtTime(ms){if(!ms)return "";var x=new Date(ms);
    return x.getFullYear()+"-"+pad(x.getMonth()+1)+"-"+pad(x.getDate())+" "+pad(x.getHours())+":"+pad(x.getMinutes())+":"+pad(x.getSeconds());}
  function truncMid(s,n){if(s.length<=n)return s;var h=Math.floor((n-1)/2);return s.slice(0,h)+"\u2026"+s.slice(-(n-1-h));}
  function preview(v){var s=String(v).replace(/\s+/g," ").trim();return s.length>120?s.slice(0,120)+"\u2026":s;}
  function pretty(v){try{return JSON.stringify(JSON.parse(v),null,2);}catch(e){return String(v);}}

  function tokenize(s,isMatch){
    if(!s)return "";
    if(s.charAt(0)==="\""){
      var i=1;
      while(i<s.length){if(s.charAt(i)==="\\"){i+=2;continue;}if(s.charAt(i)==="\""){break;}i++;}
      var tok=s.slice(0,i+1),tail=s.slice(i+1);
      return '<span class="'+(isMatch?"match":"s")+'">'+esc(tok)+"</span>"+esc(tail);
    }
    var m;
    if((m=s.match(/^-?\d+(\.\d+)?([eE][+-]?\d+)?/))){
      return '<span class="n">'+esc(m[0])+"</span>"+esc(s.slice(m[0].length));
    }
    if((m=s.match(/^(true|false|null)/))){
      return '<span class="l">'+esc(m[0])+"</span>"+esc(s.slice(m[0].length));
    }
    return esc(s);
  }

  function jsonLine(line,key,val){
    var ind=(line.match(/^\s*/)||[""])[0];
    var body=line.slice(ind.length);
    if(!body)return "";
    var km=body.match(/^("(?:[^"\\]|\\.)*")(\s*:\s*)([\s\S]*)$/);
    if(km){
      var name="";
      try{name=JSON.parse(km[1]);}catch(e){}
      var rest=km[3];
      var hit=key&&name.split(".").pop()===key&&(val===""||rest.indexOf(val)>=0);
      return esc(ind)+'<span class="'+(hit?"match":"k")+'">'+esc(km[1])+"</span>"+esc(km[2])+tokenize(rest,hit);
    }
    return esc(ind)+tokenize(body,false);
  }

  function renderJSON(v,mf,mv){
    var key=mf?mf.split(".").pop():"";
    var prettyV=v;
    try{prettyV=JSON.stringify(JSON.parse(v),null,2);}catch(e){}
    var html=prettyV.split("\n").map(function(l){return jsonLine(l,key,mv||"");}).join("\n");
    return '<pre class="json">'+html+"</pre>";
  }

  async function copyText(t){
    try{await navigator.clipboard.writeText(t);return true;}catch(e){}
    var ta=d.createElement("textarea");
    ta.value=t;ta.setAttribute("readonly","");
    ta.style.position="fixed";ta.style.top="-1000px";
    d.body.appendChild(ta);ta.select();
    var ok=false;try{ok=d.execCommand("copy");}catch(e){}
    ta.remove();return ok;
  }
  function flashCopy(btn,text){
    var orig=btn.getAttribute("data-label")||btn.textContent;
    btn.setAttribute("data-label",orig);
    copyText(text).then(function(){
      btn.textContent="Copied";
      setTimeout(function(){btn.textContent=orig;},1500);
    });
  }

  function stopStrip(){
    stripEl.hidden=true;stripEl.innerHTML="";stripEl.removeAttribute("data-mode");
  }
  function showIndeterminate(){
    if(stripEl.getAttribute("data-mode")==="ind")return;
    stripEl.hidden=false;stripEl.setAttribute("data-mode","ind");
    stripEl.innerHTML='<div class="strip-ind"><b></b></div>';
  }
  function renderProgress(p){
    scanned=p.scanned||0;
    var parts=(p.partitions||[]).filter(function(x){return x.total>0;});
    if(!parts.length){showIndeterminate();return;}
    if(stripEl.getAttribute("data-mode")!=="parts"||stripEl.children.length!==parts.length){
      stripEl.innerHTML="";stripEl.hidden=false;stripEl.setAttribute("data-mode","parts");
      parts.forEach(function(){var el=d.createElement("div");el.className="strip-seg";el.innerHTML="<i></i>";stripEl.appendChild(el);});
    }
    parts.forEach(function(s,i){
      var bar=stripEl.children[i].firstChild;
      bar.style.width=Math.min(100,Math.round(100*s.scanned/s.total))+"%";
    });
  }

  function addMatch(m){
    matches.push(m);
    var li=d.createElement("li");
    var btn=d.createElement("button");
    btn.type="button";btn.className="row";btn.setAttribute("aria-expanded","false");
    btn.innerHTML='<span class="po"><b>p'+m.partition+"</b> \u00b7 "+Number(m.offset).toLocaleString("en-US")+"</span>"+
      '<span class="tm">'+esc(fmtTime(m.timestamp_ms))+"</span>"+
      '<span class="ky">'+(m.key?esc(truncMid(m.key,18)):"")+"</span>"+
      '<span class="pv">'+esc(preview(m.value||""))+"</span>";
    var body=d.createElement("div");
    body.className="body";body.hidden=true;
    var copyBtn=d.createElement("button");
    copyBtn.type="button";copyBtn.className="btn small copy";copyBtn.textContent="Copy";
    copyBtn.addEventListener("click",function(ev){ev.stopPropagation();flashCopy(copyBtn,pretty(m.value||""));});
    body.appendChild(copyBtn);
    body.insertAdjacentHTML("beforeend",renderJSON(m.value||"",m.matched_field,m.matched_value));
    btn.addEventListener("click",function(){
      var open=btn.getAttribute("aria-expanded")==="true";
      btn.setAttribute("aria-expanded",String(!open));
      body.hidden=open;
    });
    li.appendChild(btn);li.appendChild(body);
    resultsEl.appendChild(li);
  }

  function setNotice(html){
    resultsEl.innerHTML="";
    if(!html){noticeEl.hidden=true;noticeEl.innerHTML="";return;}
    noticeEl.hidden=false;noticeEl.innerHTML=html;
  }
  function idle(){
    setNotice('<p>Enter a topic and a filter, then press <kbd>Enter</kbd>.</p><p><code>value.id == "abc123"</code></p>');
    statusText.textContent="";
  }
  function noMatch(info){
    var topic=topicEl.value.trim();
    var filter=filterEl.value.trim();
    var mode=startEl.value;
    var read=Number((info&&info.scanned)||0).toLocaleString("en-US");
    var where=mode==="from-start"?"from the start":"from the end";
    setNotice('<h2>No messages matched</h2><p>Nothing in '+esc(topic)+" matched <code>"+esc(filter)+"</code> in the "+esc(read)+" messages read "+esc(where)+". Raise the limit or change Start.</p>");
  }
  function fail(kind,message){
    var head={broker:"Can\u2019t reach the Kafka service",topic:"Topic not found",filter:"Filter is not valid"}[kind]||"Search failed";
    var tip={broker:"Check the broker address and that Kafka is running.",
             topic:"Check the topic name and that it exists on the cluster.",
             filter:"Fix the expression and run the search again."}[kind]||"Run the search again.";
    setNotice('<h2 class="error">'+esc(head)+"</h2><p>"+esc(tip)+
      '</p><details><summary>Error detail</summary><pre>'+esc(message||"")+"</pre></details>");
    statusText.textContent="Search failed";
  }

  var LS="kafkagrep.search";
  function savePrefs(){
    try{localStorage.setItem(LS,JSON.stringify({topic:topicEl.value,filter:filterEl.value,start:startEl.value,since:sinceEl.value,limit:limitEl.value}));}catch(e){}
  }
  function loadPrefs(){
    try{
      var p=JSON.parse(localStorage.getItem(LS)||"{}");
      if(p.topic)topicEl.value=p.topic;
      if(p.filter)filterEl.value=p.filter;
      if(p.start)startEl.value=p.start;
      if(p.since)sinceEl.value=p.since;
      if(p.limit)limitEl.value=p.limit;
    }catch(e){}
  }

  function closeES(){if(es){es.close();es=null;}}
  function setRunning(on){
    running=on;
    goBtn.textContent=on?"Stop":"Search";
    goBtn.className="btn "+(on?"stop":"primary");
    goBtn.setAttribute("aria-label",on?"Stop the search":"Search");
  }

  function startSearch(){
    if(running)return;
    savePrefs();
    setNotice("");
    resultsEl.innerHTML="";
    matches=[];scanned=0;
    setRunning(true);
    statusText.innerHTML="Searching "+esc(topicEl.value.trim()||"topics")+"\u2026";
    copyAllBtn.hidden=true;
    d.title="kafkagrep \u00b7 "+topicEl.value.trim();
    stripEl.innerHTML='<div class="strip-ind"><b></b></div>';
    stripEl.hidden=false;stripEl.setAttribute("data-mode","ind");
    var params=new URLSearchParams();
    params.set("topics",topicEl.value.trim());
    params.set("q",filterEl.value.trim());
    params.set("mode",startEl.value);
    var since=sinceEl.value.trim();
    if(since){params.set("since",since);}
    params.set("limit",limitEl.value||"100");
    es=new EventSource("/api/search?"+params.toString());
    es.addEventListener("match",function(e){
      try{addMatch(JSON.parse(e.data));}catch(err){}
    });
    es.addEventListener("progress",function(e){
      try{renderProgress(JSON.parse(e.data));}catch(err){}
    });
    es.addEventListener("done",function(e){
      var info={};
      try{info=JSON.parse(e.data);}catch(err){}
      closeES();setRunning(false);stopStrip();
      var secs=((info.elapsed_ms||0)/1000).toFixed(1);
      statusText.textContent=matches.length+(matches.length===1?" match":" matches")+
        " \u00b7 scanned "+Number(info.scanned||0).toLocaleString("en-US")+
        " messages \u00b7 "+secs+"s";
      copyAllBtn.hidden=matches.length===0;
      if(matches.length===0){noMatch(info);}
      else if(matches.length===1){var first=resultsEl.querySelector(".row");if(first)first.click();}
    });
    es.addEventListener("error",function(e){
      if(e&&e.data){
        var info={};try{info=JSON.parse(e.data);}catch(err){}
        closeES();setRunning(false);stopStrip();
        fail(info.kind||"internal",info.message||"");
        return;
      }
      if(!running)return;
      closeES();setRunning(false);stopStrip();
      if(matches.length){statusText.textContent="Stream ended \u00b7 "+matches.length+(matches.length===1?" match":" matches");}
      else{fail("broker","the connection to the search stream ended unexpectedly");}
    });
  }

  function stopSearch(){
    if(!running)return;
    closeES();setRunning(false);stopStrip();
    statusText.textContent="Stopped after "+scanned.toLocaleString("en-US")+" messages.";
  }

  form.addEventListener("submit",function(e){e.preventDefault();startSearch();});
  goBtn.addEventListener("click",function(e){
    if(running){e.preventDefault();stopSearch();}
  });
  copyAllBtn.addEventListener("click",function(){
    var text=matches.map(function(m){return m.value;}).join("\n");
    var orig=copyAllBtn.textContent;
    copyText(text).then(function(){copyAllBtn.textContent="Copied";setTimeout(function(){copyAllBtn.textContent=orig;},1500);});
  });
  d.addEventListener("keydown",function(e){
    if(e.key==="Escape"&&running){e.preventDefault();stopSearch();return;}
    if(e.key==="/"){
      var t=e.target&&e.target.tagName;
      if(t==="INPUT"||t==="SELECT"||t==="TEXTAREA")return;
      e.preventDefault();filterEl.focus();
    }
  });

  loadPrefs();
  idle();
})();
</script>
</body>
</html>`
