// Package openapi sirve la especificación OpenAPI embebida y una UI de
// Swagger autocontenida (ADR-10: el contrato es la fuente de verdad y vive
// en backend/api/openapi.yaml, embebido en el binario).
package openapi

import (
	"bytes"
	"encoding/json"
	"net/http"
	"sync"

	"gopkg.in/yaml.v3"

	"tiendas-track/backend/api"
)

var (
	jsonOnce sync.Once
	jsonSpec []byte
	jsonErr  error
)

// SpecYAML devuelve la especificación original.
func SpecYAML() []byte { return api.SpecYAML }

// SpecJSON convierte la especificación YAML a JSON (se cachea en memoria).
func SpecJSON() ([]byte, error) {
	jsonOnce.Do(func() {
		var doc any
		if jsonErr = yaml.Unmarshal(api.SpecYAML, &doc); jsonErr != nil {
			return
		}
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetIndent("", "  ")
		jsonErr = enc.Encode(doc)
		jsonSpec = buf.Bytes()
	})
	return jsonSpec, jsonErr
}

// Register monta los endpoints de documentación en el mux:
//
//	GET /api/v1/openapi.yaml
//	GET /api/v1/openapi.json
//	GET /swagger/index.html
func Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/openapi.yaml", serveYAML)
	mux.HandleFunc("GET /api/v1/openapi.json", serveJSON)
	mux.HandleFunc("GET /swagger", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/swagger/index.html", http.StatusFound)
	})
	mux.HandleFunc("GET /swagger/index.html", serveUI)
	mux.HandleFunc("GET /swagger/openapi.json", serveJSON)
}

func serveYAML(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	_, _ = w.Write(api.SpecYAML)
}

func serveJSON(w http.ResponseWriter, _ *http.Request) {
	body, err := SpecJSON()
	if err != nil {
		http.Error(w, `{"error":{"code":"INTERNAL","message":"spec inválida"}}`, http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = w.Write(body)
}

// serveUI responde una UI de Swagger mínima y autocontenida que apunta a la
// especificación embebida. No depende de CDNs externos: renderiza el spec con
// JavaScript propio, suficiente para explorar y probar la API sin internet.
func serveUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(uiHTML))
}

const uiHTML = `<!doctype html>
<html lang="es">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Tiendas-Track API — OpenAPI 3.1</title>
<style>
  :root { color-scheme: light dark; }
  * { box-sizing: border-box; }
  body { margin:0; font:14px/1.5 system-ui,-apple-system,Segoe UI,Roboto,sans-serif;
         display:flex; min-height:100vh; }
  nav { width:320px; border-right:1px solid #8884; padding:12px; overflow-y:auto; }
  main { flex:1; padding:16px 24px; overflow-y:auto; }
  h1 { font-size:16px; margin:0 0 12px; }
  .tag { font-weight:700; margin:14px 0 4px; text-transform:uppercase; font-size:11px; opacity:.7; }
  .op { display:flex; gap:8px; align-items:center; padding:5px 8px; border-radius:6px;
        cursor:pointer; text-decoration:none; color:inherit; }
  .op:hover, .op.active { background:#8881; }
  .m { font:700 10px/1.6 ui-monospace,monospace; border-radius:4px; padding:0 6px;
       color:#fff; min-width:52px; text-align:center; }
  .get{background:#2563eb}.post{background:#16a34a}.patch{background:#d97706}.delete{background:#dc2626}
  .path { font-family:ui-monospace,monospace; font-size:12px; opacity:.85; }
  section { border:1px solid #8883; border-radius:8px; padding:14px; margin-bottom:16px; }
  .summary { font-weight:600; margin-bottom:4px; }
  .desc { opacity:.8; white-space:pre-wrap; }
  .params, .resp { margin-top:10px; }
  table { border-collapse:collapse; width:100%; font-size:13px; }
  th,td { text-align:left; padding:4px 8px; border-bottom:1px solid #8882; vertical-align:top; }
  code,pre { font-family:ui-monospace,monospace; font-size:12px; }
  pre { background:#8881; padding:8px; border-radius:6px; overflow-x:auto; }
  .badge { display:inline-block; font:700 11px/1.6 ui-monospace,monospace; padding:0 8px;
           border-radius:10px; color:#fff; margin-right:6px; }
  .auth { border-top:1px solid #8883; margin-top:12px; padding-top:12px; }
  .auth input, .param-input, textarea { width:100%; padding:7px; border-radius:5px; border:1px solid #8886; font:12px/1.4 ui-monospace,monospace; }
  textarea { min-height:120px; resize:vertical; }
  .try-btn { border:0; border-radius:5px; padding:7px 12px; background:#2563eb; color:white; cursor:pointer; font-weight:600; }
  .try-btn:disabled { opacity:.6; cursor:wait; }
  .result { margin-top:10px; }
  @media (max-width:760px){ nav{width:100%;border-right:none} body{flex-direction:column} }
</style>
</head>
<body>
<nav><h1>Tiendas-Track API<br><small style="font-weight:400;opacity:.7">OpenAPI 3.1 — contrato canónico (ADR-10)</small></h1><div id="index"></div>
<div class="auth"><strong>Autorización</strong><br><label style="font-size:12px">Token Bearer</label><input id="bearer-token" autocomplete="off" placeholder="pega aquí el access_token"><button class="try-btn" id="save-token" style="margin-top:6px">Guardar token</button></div>
<p style="font-size:12px;opacity:.6">Spec: <a href="/api/v1/openapi.yaml">yaml</a> · <a href="/api/v1/openapi.json">json</a></p></nav>
<main id="detail">Cargando especificación…</main>
<script>
const METHODS=['get','post','put','patch','delete'];
fetch('/swagger/openapi.json').then(r=>r.json()).then(spec=>{
  const idx=document.getElementById('index'), det=document.getElementById('detail');
  det.innerHTML='';
  const ops=[];
  for(const [p,item] of Object.entries(spec.paths||{})){
    for(const m of METHODS){
      if(!item[m]) continue;
      ops.push({p,m,op:item[m]});
    }
  }
  ops.forEach((it,i)=>{
    const a=document.createElement('a');
    a.className='op'; a.href='#op'+i;
    a.innerHTML='<span class="m '+it.m+'">'+it.m.toUpperCase()+'</span><span class="path">'+it.p+'</span>';
    idx.appendChild(a);

    const s=document.createElement('section'); s.id='op'+i;
    const op=it.op;
    let html='<span class="badge m '+it.m+'">'+it.m.toUpperCase()+'</span><code>'+it.p+'</code>';
    html+='<div class="summary">'+(op.summary||'')+'</div>';
    if(op.description) html+='<div class="desc">'+op.description.trim()+'</div>';
    if((op.security||[]).some(x=>x.bearerAuth)) html+='<p style="font-size:12px">🔒 Requiere <code>Authorization: Bearer &lt;token&gt;</code></p>';

    const params=(op.parameters||[]).concat((spec.paths[it.p].parameters||[]));
    if(params.length){
      html+='<div class="params"><table><tr><th>Parámetro</th><th>En</th><th>Tipo</th><th>Requerido</th><th>Valor</th></tr>';
      params.forEach(p=>{
        const ref=p.$ref?refName(p.$ref):null;
        const q=ref?((spec.components.parameters||{})[ref]||{}):p;
        const sch=q.schema||{};
        const example=q.example!==undefined?q.example:sch.example;
        const description=q.description?'<br><small>'+esc(q.description.replace(/\n/g,' ').replace(/\x60([^\x60]+)\x60/g,'$1'))+'</small>':'';
        const placeholder=example!==undefined?' placeholder="'+esc(String(example))+'"':'';
        html+='<tr><td><code>'+esc(q.name||ref||'')+'</code>'+description+'</td><td>'+esc(q.in||'')+'</td><td>'+ 
          (sch.type||(sch.$ref?refName(sch.$ref):'')||'')+(sch.enum?' ['+sch.enum.join(' | ')+']':'')+
          '</td><td>'+ (q.required?'sí':'no')+'</td><td><input class="param-input" data-param-name="'+esc(q.name||ref||'')+'" data-param-in="'+esc(q.in||'')+'"'+placeholder+(q.required?' required':'')+'></td></tr>';
      });
      html+='</table></div>';
    }
    if(op.requestBody){
      const media=op.requestBody.content&&op.requestBody.content['application/json'];
      if(media){
        html+='<div class="resp"><strong>Body JSON</strong> '+(op.requestBody.required?'(requerido)':'')+'<textarea id="body-'+i+'">'+
          esc(JSON.stringify(schemaExample(media.schema,spec),null,2))+'</textarea></div>';
      }
    }
    if(op.responses){
      html+='<div class="resp"><table><tr><th>Estado</th><th>Descripción</th></tr>';
      for(const [code,r] of Object.entries(op.responses)){
        let d=r.description||'';
        if(r.$ref) d=(((spec.components.responses||{})[refName(r.$ref)]||{}).description)||r.$ref;
        html+='<tr><td><code>'+code+'</code></td><td>'+d+'</td></tr>';
      }
      html+='</table></div>';
    }
    html+='<div class="resp"><button class="try-btn execute-btn" data-op="'+i+'">Try it out</button><div id="result-'+i+'" class="result"></div></div>';
    s.innerHTML=html;
    det.appendChild(s);
  });

  document.getElementById('save-token').addEventListener('click',()=>{
    localStorage.setItem('tiendas_bearer_token',document.getElementById('bearer-token').value.trim());
  });
  document.getElementById('bearer-token').value=localStorage.getItem('tiendas_bearer_token')||'';
  document.querySelectorAll('.execute-btn').forEach(button=>button.addEventListener('click',()=>executeOperation(button.dataset.op,spec)));

  async function executeOperation(index,spec){
    const operation=ops[Number(index)], section=document.getElementById('op'+index);
    let path=operation.p;
    const query=new URLSearchParams(), headers={};
    section.querySelectorAll('.param-input').forEach(input=>{
      const value=input.value.trim();
      if(!value) return;
      if(input.dataset.paramIn==='path') path=path.replace('{'+input.dataset.paramName+'}',encodeURIComponent(value));
      else if(input.dataset.paramIn==='query') query.set(input.dataset.paramName,value);
      else if(input.dataset.paramIn==='header') headers[input.dataset.paramName]=value;
    });
    const missing=[...section.querySelectorAll('.param-input[required]')].filter(input=>!input.value.trim()).map(input=>input.dataset.paramName);
    if(missing.length){ showResult(index,'Faltan parámetros requeridos: '+missing.join(', ')); return; }
    const qs=query.toString();
    const options={method:operation.m.toUpperCase(),headers};
    const token=document.getElementById('bearer-token').value.trim();
    if(token) headers.Authorization='Bearer '+token.replace(/^Bearer\s+/i,'');
    const body=document.getElementById('body-'+index);
    if(body){
      try{ JSON.parse(body.value); }catch(e){ showResult(index,'JSON inválido: '+e.message); return; }
      headers['Content-Type']='application/json'; options.body=body.value;
    }
    const result=document.getElementById('result-'+index); result.innerHTML='Ejecutando…';
    try{
      const response=await fetch(path+(qs?'?'+qs:''),options);
      const text=await response.text();
      showResult(index,response.status+' '+response.statusText+'\n\n'+text);
    }catch(e){ showResult(index,'Error de conexión: '+e.message); }
  }

  function showResult(index,text){
    document.getElementById('result-'+index).innerHTML='<strong>Response</strong><pre>'+esc(text)+'</pre>';
  }

  function refName(ref){ return ref.split('/').pop(); }
  function esc(s){ return s.replace(/[&<>]/g,c=>({'&':'&amp;','<':'&lt;','>':'&gt;'}[c])); }
  function schemaExample(sch,spec){
    if(!sch||typeof sch!=='object') return {};
    if(sch.$ref){
      const name=refName(sch.$ref);
      return schemaExample(((spec.components||{}).schemas||{})[name],spec);
    }
    if(sch.example!==undefined) return sch.example;
    if(sch.default!==undefined) return sch.default;
    if(sch.enum) return sch.enum[0];
    switch(sch.type){
      case 'object':{
        const o={};
        for(const [k,v] of Object.entries(sch.properties||{})) o[k]=schemaExample(v,spec);
        return o;
      }
      case 'array': return [schemaExample(sch.items,spec)];
      case 'integer': case 'number': return 0;
      case 'boolean': return true;
      case 'string':
        if(sch.format==='uuid') return '00000000-0000-0000-0000-000000000000';
        if(sch.format==='date-time') return new Date().toISOString();
        if(sch.format==='email') return 'usuario@example.com';
        return 'string';
      default: return null;
    }
  }
}).catch(e=>{ document.getElementById('detail').textContent='No se pudo cargar la especificación: '+e; });
</script>
</body>
</html>`
