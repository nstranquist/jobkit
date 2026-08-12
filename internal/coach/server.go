package coach

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"
)

type Server struct {
	Store     *Store
	Providers *ProviderConfig
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /", s.handleIndex)
	mux.HandleFunc("GET /api/config", s.handleConfig)
	mux.HandleFunc("GET /api/decks", s.handleDecks)
	mux.HandleFunc("GET /api/decks/{id}", s.handleDeck)
	mux.HandleFunc("GET /api/stats", s.handleStats)
	mux.HandleFunc("POST /api/sessions", s.handleSession)
	mux.HandleFunc("POST /api/transcribe", s.handleTranscribe)
	return securityHeaders(localRequestBoundary(mux))
}

func (s *Server) Serve(ctx context.Context, addr string) error {
	if err := validateLoopbackAddress(addr); err != nil {
		return err
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		return err
	}
	server := &http.Server{
		Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout: 30 * time.Second, WriteTimeout: 60 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	done := make(chan error, 1)
	go func() { done <- server.Serve(listener) }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			return err
		}
		return nil
	case err := <-done:
		if err == http.ErrServerClosed {
			return nil
		}
		return err
	}
}

func validateLoopbackAddress(addr string) error {
	host, _, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("invalid coach address %q: %w", addr, err)
	}
	if host == "localhost" {
		return nil
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return fmt.Errorf("coach address must use localhost or a loopback IP")
	}
	return nil
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'unsafe-inline'; style-src 'unsafe-inline'; connect-src 'self'; media-src 'self' blob:; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		next.ServeHTTP(w, r)
	})
}

// localRequestBoundary rejects DNS-rebinding hosts. Browser writes must also
// come from the exact origin that served the Coach UI. Non-browser clients do
// not send Origin and can continue to use the loopback API directly.
func localRequestBoundary(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !loopbackHost(r.Host) {
			writeAPIError(w, http.StatusForbidden, fmt.Errorf("coach request host must be localhost or a loopback IP"))
			return
		}
		if origin := strings.TrimSpace(r.Header.Get("Origin")); origin != "" && r.Method != http.MethodGet && r.Method != http.MethodHead && r.Method != http.MethodOptions {
			parsed, err := url.Parse(origin)
			if err != nil || parsed.Scheme == "" || !strings.EqualFold(parsed.Host, r.Host) {
				writeAPIError(w, http.StatusForbidden, fmt.Errorf("coach browser request must use the same localhost origin"))
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func loopbackHost(hostPort string) bool {
	host := strings.TrimSpace(hostPort)
	if split, _, err := net.SplitHostPort(host); err == nil {
		host = split
	} else if strings.Contains(host, ":") {
		return false
	}
	host = strings.Trim(host, "[]")
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = io.WriteString(w, indexHTML)
}

func (s *Server) handleConfig(w http.ResponseWriter, _ *http.Request) {
	providers := []string{}
	if s.Providers != nil {
		for name := range s.Providers.Providers {
			providers = append(providers, name)
		}
		sort.Strings(providers)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"schema_version":          SchemaVersion,
		"transcription_available": s.Providers != nil && s.Providers.Transcriber != nil,
		"providers":               providers,
	})
}

func (s *Server) handleDecks(w http.ResponseWriter, _ *http.Request) {
	decks, err := s.Store.ListDecks()
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"decks": decks})
}

func (s *Server) handleDeck(w http.ResponseWriter, r *http.Request) {
	deck, err := s.Store.LoadDeck(r.PathValue("id"))
	if err != nil {
		if os.IsNotExist(err) {
			writeAPIError(w, http.StatusNotFound, err)
			return
		}
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	writeJSON(w, http.StatusOK, deck)
}

func (s *Server) handleStats(w http.ResponseWriter, r *http.Request) {
	report, err := s.Store.Stats(time.Now().UTC(), r.URL.Query().Get("project"))
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

type sessionRequest struct {
	DeckID   string   `json:"deck_id"`
	Answers  []Answer `json:"answers"`
	Provider string   `json:"provider,omitempty"`
	Useful   *bool    `json:"useful,omitempty"`
}

func (s *Server) handleSession(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	var request sessionRequest
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(&request); err != nil {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("decode session request: %w", err))
		return
	}
	if err := dec.Decode(&struct{}{}); err != io.EOF {
		writeAPIError(w, http.StatusBadRequest, fmt.Errorf("decode session request: trailing JSON data"))
		return
	}
	deck, err := s.Store.LoadDeck(request.DeckID)
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	bundle, err := s.Store.LoadSource()
	if err != nil {
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if err := ValidateSessionInput(deck, bundle, request.Answers); err != nil {
		writeAPIError(w, http.StatusConflict, err)
		return
	}
	now := time.Now().UTC()
	session := Evaluate(deck, bundle, request.Answers, now, now)
	session.Useful = request.Useful
	if request.Provider != "" && request.Provider != "none" {
		if s.Providers == nil {
			session.ProviderError = "coach provider configuration is missing"
		} else if feedback, feedbackErr := RunFeedback(r.Context(), s.Providers, request.Provider, bundle, deck, session); feedbackErr != nil {
			session.ProviderError = feedbackErr.Error()
		} else {
			session.Feedback = feedback
		}
	}
	if err := s.Store.AppendSession(session); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusCreated, session)
}

func (s *Server) handleTranscribe(w http.ResponseWriter, r *http.Request) {
	if s.Providers == nil || s.Providers.Transcriber == nil {
		writeAPIError(w, http.StatusNotImplemented, fmt.Errorf("coach transcriber is not configured"))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 20*1024*1024)
	temp, err := os.CreateTemp(s.Store.Root, ".coach-audio-*.wav")
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	path := temp.Name()
	defer os.Remove(path)
	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	if _, err := io.Copy(temp, r.Body); err != nil {
		temp.Close()
		writeAPIError(w, http.StatusBadRequest, err)
		return
	}
	if err := temp.Close(); err != nil {
		writeAPIError(w, http.StatusInternalServerError, err)
		return
	}
	text, err := RunTranscriber(r.Context(), s.Providers, path)
	if err != nil {
		writeAPIError(w, http.StatusBadGateway, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"text": text})
}

func writeAPIError(w http.ResponseWriter, status int, err error) {
	writeJSON(w, status, map[string]any{"error": map[string]string{"message": err.Error()}})
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

const indexHTML = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width,initial-scale=1">
  <title>JobKit Coach</title>
  <style>
    :root{color-scheme:dark;--bg:#0b1020;--panel:#141b30;--line:#2d3858;--text:#edf2ff;--muted:#aeb9d4;--accent:#7dd3fc;--good:#86efac}
    *{box-sizing:border-box}body{margin:0;background:radial-gradient(circle at 15% 0,#192445,var(--bg) 42%);color:var(--text);font:16px/1.55 ui-sans-serif,system-ui;min-height:100vh}
    main{max-width:920px;margin:auto;padding:32px 20px 80px}h1{font-size:clamp(2rem,5vw,3.5rem);margin:.2rem 0}h2{margin-top:2rem}.lead,.meta{color:var(--muted)}
    .toolbar,.card{background:color-mix(in srgb,var(--panel) 94%,transparent);border:1px solid var(--line);border-radius:16px;padding:18px;margin:16px 0;box-shadow:0 18px 50px #0005}
    .toolbar{display:flex;gap:12px;align-items:end;flex-wrap:wrap}label{display:grid;gap:6px;flex:1;min-width:220px}select,textarea,button{font:inherit}select,textarea{color:var(--text);background:#0c1327;border:1px solid var(--line);border-radius:10px;padding:10px}textarea{width:100%;min-height:150px;resize:vertical}
    button{border:0;border-radius:10px;padding:10px 15px;background:var(--accent);color:#082032;font-weight:750;cursor:pointer}button.secondary{background:#263453;color:var(--text)}button:disabled{opacity:.45;cursor:not-allowed}
    .question{margin:26px 0}.prompt{font-size:1.1rem;font-weight:700}.rubric{font-size:.9rem;color:var(--muted)}.result{border-left:4px solid var(--good);padding-left:14px}.hidden{display:none}.status{min-height:1.5em;color:var(--muted)}code{color:var(--accent)}
  </style>
</head>
<body><main>
  <p class="meta">LOCALHOST ONLY · EVIDENCE-LINKED PRACTICE</p>
  <h1>JobKit Coach</h1>
  <p class="lead">Practice project, behavioral, system-design, and claim-defense answers. Deterministic scoring remains authoritative.</p>
  <section class="toolbar">
    <label>Practice deck<select id="deck"></select></label>
    <label>Advisory feedback<select id="provider"><option value="none">Deterministic score only</option></select></label>
    <button id="load">Load deck</button>
    <button id="stats" class="secondary">View stats</button>
  </section>
  <p id="status" class="status" aria-live="polite"></p>
  <section id="workspace"></section>
  <section id="result"></section>
</main>
<script>
const deckSelect=document.querySelector('#deck'),providerSelect=document.querySelector('#provider'),workspace=document.querySelector('#workspace'),result=document.querySelector('#result'),statusEl=document.querySelector('#status');
let currentDeck=null,canTranscribe=false,recording=null;
function status(text){statusEl.textContent=text}
async function api(path,options={}){const response=await fetch(path,options);const body=await response.json();if(!response.ok)throw new Error(body.error?.message||response.statusText);return body}
async function boot(){const [decks,config]=await Promise.all([api('/api/decks'),api('/api/config')]);canTranscribe=config.transcription_available;deckSelect.replaceChildren(...decks.decks.map(d=>new Option(d.role+' · '+d.mode+' · '+d.minutes+' min',d.id)));providerSelect.append(...(config.providers||[]).map(name=>new Option(name,name)));if(!decks.decks.length)status('Create a deck with jobkit coach deck, then reload this page.')}
function text(tag,value,className){const node=document.createElement(tag);node.textContent=value;if(className)node.className=className;return node}
async function loadDeck(){if(!deckSelect.value)return;currentDeck=await api('/api/decks/'+encodeURIComponent(deckSelect.value));workspace.replaceChildren();result.replaceChildren();workspace.append(text('h2',currentDeck.role+' practice'));workspace.append(text('p',currentDeck.questions.length+' questions · '+currentDeck.minutes+' minutes · '+currentDeck.mode,'meta'));
  currentDeck.questions.forEach((q,index)=>{const card=document.createElement('article');card.className='card question';card.append(text('p',(index+1)+'. '+q.prompt,'prompt'));card.append(text('p','Target: '+Math.round(q.time_seconds/60)+' minutes · Evidence: '+((q.evidence_ids||[]).join(', ')||'source bundle'),'rubric'));const area=document.createElement('textarea');area.id='answer-'+q.id;area.placeholder='Type your answer. Name decisions, tradeoffs, evidence, and boundaries.';card.append(area);if(canTranscribe){const record=document.createElement('button');record.className='secondary';record.textContent='Record answer';record.addEventListener('click',()=>toggleRecord(record,area).catch(e=>status(e.message)));card.append(record)}workspace.append(card)});
  const submit=document.createElement('button');submit.textContent='Score session';submit.addEventListener('click',()=>scoreSession().catch(e=>status(e.message)));workspace.append(submit);status('Deck loaded. Raw answers stay in your local JobKit state.')}
async function scoreSession(){const provider=providerSelect.value;if(provider.endsWith('-hosted')&&!window.confirm('Send this practice request and your answer text to '+provider+'?')){status('Hosted feedback canceled. Your answers were not submitted.');return}const answers=currentDeck.questions.map(q=>({question_id:q.id,text:document.querySelector('#answer-'+q.id).value}));status('Scoring session…');const session=await api('/api/sessions',{method:'POST',headers:{'Content-Type':'application/json'},body:JSON.stringify({deck_id:currentDeck.id,answers,provider})});result.replaceChildren();const card=document.createElement('article');card.className='card result';card.append(text('h2','Score '+session.score+'/100'));card.append(text('p',session.claim_violations+' claim violations · next review '+new Date(session.next_review_at).toLocaleDateString()));session.results.forEach(row=>card.append(text('p',row.question_id+': '+row.score.total+'/100 · missing '+((row.missing_concepts||[]).join(', ')||'none'))));if(session.feedback)card.append(text('p','Advisory feedback ('+session.feedback.provider+'): '+session.feedback.summary));if(session.provider_error)card.append(text('p','Advisory provider unavailable: '+session.provider_error,'meta'));result.append(card);status('Session saved locally.')}
async function showStats(){const report=await api('/api/stats');workspace.replaceChildren();result.replaceChildren();const card=document.createElement('article');card.className='card';card.append(text('h2','Practice stats'));card.append(text('p',report.sessions+' sessions · average '+(report.average_score||0)+'/100 · '+report.due_reviews+' due reviews'));Object.entries(report.by_project||{}).forEach(([name,band])=>card.append(text('p',name+': '+band.average+'/100 across '+band.answers+' answers')));workspace.append(card);status('Stats use append-only local sessions.')}
async function toggleRecord(button,area){if(recording){if(recording.button!==button){status('Stop the current recording before you record another answer.');return}button.disabled=true;const wav=await stopRecording();status('Transcribing local audio…');try{const response=await api('/api/transcribe',{method:'POST',headers:{'Content-Type':'audio/wav'},body:wav});area.value=(area.value+' '+response.text).trim();status('Local transcript added.')}finally{button.disabled=false;button.textContent='Record answer'}return}recording=await startRecording();recording.button=button;button.textContent='Stop and transcribe';status('Recording on this device…')}
async function startRecording(){const stream=await navigator.mediaDevices.getUserMedia({audio:true});const context=new AudioContext();const source=context.createMediaStreamSource(stream);const processor=context.createScriptProcessor(4096,1,1);const chunks=[];processor.onaudioprocess=e=>chunks.push(new Float32Array(e.inputBuffer.getChannelData(0)));const mute=context.createGain();mute.gain.value=0;source.connect(processor);processor.connect(mute);mute.connect(context.destination);return{stream,context,source,processor,chunks,sampleRate:context.sampleRate}}
async function stopRecording(){const state=recording;recording=null;state.processor.disconnect();state.source.disconnect();state.stream.getTracks().forEach(t=>t.stop());await state.context.close();let size=state.chunks.reduce((n,c)=>n+c.length,0),samples=new Float32Array(size),offset=0;state.chunks.forEach(c=>{samples.set(c,offset);offset+=c.length});return encodeWav(samples,state.sampleRate)}
function encodeWav(samples,rate){const buffer=new ArrayBuffer(44+samples.length*2),view=new DataView(buffer);const word=(o,s)=>[...s].forEach((c,i)=>view.setUint8(o+i,c.charCodeAt(0)));word(0,'RIFF');view.setUint32(4,36+samples.length*2,true);word(8,'WAVE');word(12,'fmt ');view.setUint32(16,16,true);view.setUint16(20,1,true);view.setUint16(22,1,true);view.setUint32(24,rate,true);view.setUint32(28,rate*2,true);view.setUint16(32,2,true);view.setUint16(34,16,true);word(36,'data');view.setUint32(40,samples.length*2,true);samples.forEach((s,i)=>view.setInt16(44+i*2,Math.max(-1,Math.min(1,s))*0x7fff,true));return new Blob([buffer],{type:'audio/wav'})}
document.querySelector('#load').addEventListener('click',()=>loadDeck().catch(e=>status(e.message));document.querySelector('#stats').addEventListener('click',()=>showStats().catch(e=>status(e.message));boot().catch(e=>status(e.message));
</script></body></html>`
