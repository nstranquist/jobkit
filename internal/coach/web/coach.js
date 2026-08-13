"use strict";
const deckSelect = document.querySelector("#deck");
const providerSelect = document.querySelector("#provider");
const workspace = document.querySelector("#workspace");
const result = document.querySelector("#result");
const statusElement = document.querySelector("#status");
let currentDeck = null;
let canTranscribe = false;
let recording = null;

function status(value) { statusElement.textContent = value; }
function text(tag, value, className) {
  const node = document.createElement(tag);
  node.textContent = value;
  if (className) node.className = className;
  return node;
}
async function api(path, options = {}) {
  const response = await fetch(path, options);
  const body = await response.json();
  if (!response.ok) throw new Error(body.error?.message || response.statusText);
  return body;
}
async function boot() {
  const [decks, config] = await Promise.all([api("/api/decks"), api("/api/config")]);
  canTranscribe = config.transcription_available;
  deckSelect.replaceChildren(...decks.decks.map((deck) => new Option(
    deck.role + " · " + deck.mode + " · " + deck.minutes + " min", deck.id
  )));
  providerSelect.append(...(config.providers || []).map((name) => new Option(name, name)));
  if (!decks.decks.length) status("Create a deck with jobkit coach deck, then reload this page.");
}
async function loadDeck() {
  if (!deckSelect.value) return;
  currentDeck = await api("/api/decks/" + encodeURIComponent(deckSelect.value));
  workspace.replaceChildren();
  result.replaceChildren();
  workspace.append(text("h2", currentDeck.role + " practice"));
  workspace.append(text("p", currentDeck.questions.length + " questions · " + currentDeck.minutes + " minutes · " + currentDeck.mode, "meta"));
  currentDeck.questions.forEach((question, index) => {
    const card = document.createElement("article");
    card.className = "card question";
    card.append(text("p", (index + 1) + ". " + question.prompt, "prompt"));
    card.append(text("p", "Target: " + Math.round(question.time_seconds / 60) + " minutes · Evidence: " + ((question.evidence_ids || []).join(", ") || "source bundle"), "rubric"));
    const area = document.createElement("textarea");
    area.id = "answer-" + question.id;
    area.placeholder = "Type your answer. Name decisions, tradeoffs, evidence, and boundaries.";
    card.append(area);
    if (canTranscribe) {
      const record = document.createElement("button");
      record.className = "secondary";
      record.textContent = "Record answer";
      record.addEventListener("click", () => toggleRecord(record, area).catch((error) => status(error.message)));
      card.append(record);
    }
    workspace.append(card);
  });
  const submit = document.createElement("button");
  submit.textContent = "Score session";
  submit.addEventListener("click", () => scoreSession().catch((error) => status(error.message)));
  workspace.append(submit);
  status("Deck loaded. Raw answers stay in your local JobKit state.");
}
async function scoreSession() {
  const provider = providerSelect.value;
  if (provider.endsWith("-hosted") && !window.confirm("Send this practice request and your answer text to " + provider + "?")) {
    status("Hosted feedback canceled. Your answers were not submitted.");
    return;
  }
  const answers = currentDeck.questions.map((question) => ({
    question_id: question.id,
    text: document.querySelector("#answer-" + question.id).value
  }));
  status("Scoring session…");
  const session = await api("/api/sessions", {
    method: "POST",
    headers: {"Content-Type": "application/json"},
    body: JSON.stringify({deck_id: currentDeck.id, answers, provider})
  });
  result.replaceChildren();
  const card = document.createElement("article");
  card.className = "card result";
  card.append(text("h2", "Score " + session.score + "/100"));
  card.append(text("p", "Rubric " + session.rubric_version + " · " + session.claim_violations + " claim violations · next review " + new Date(session.next_review_at).toLocaleDateString()));
  session.results.forEach((row) => card.append(text("p", row.question_id + ": " + row.score.total + "/100 · missing " + ((row.missing_concepts || []).join(", ") || "none"))));
  if (session.feedback) card.append(text("p", "Advisory feedback (" + session.feedback.provider + "): " + session.feedback.summary));
  if (session.provider_error) card.append(text("p", "Advisory provider unavailable: " + session.provider_error, "meta"));
  result.append(card);
  status("Session saved locally.");
}
async function showStats() {
  const report = await api("/api/stats");
  workspace.replaceChildren();
  result.replaceChildren();
  const card = document.createElement("article");
  card.className = "card";
  card.append(text("h2", "Practice stats"));
  card.append(text("p", report.sessions + " sessions · average " + (report.average_score || 0) + "/100 · " + report.due_reviews + " due reviews"));
  Object.entries(report.by_project || {}).forEach(([name, band]) => card.append(text("p", name + ": " + band.average + "/100 across " + band.answers + " answers")));
  workspace.append(card);
  status("Stats use append-only local sessions.");
}
async function toggleRecord(button, area) {
  if (recording) {
    if (recording.button !== button) {
      status("Stop the current recording before you record another answer.");
      return;
    }
    button.disabled = true;
    const wav = await stopRecording();
    status("Transcribing local audio…");
    try {
      const response = await api("/api/transcribe", {method: "POST", headers: {"Content-Type": "audio/wav"}, body: wav});
      area.value = (area.value + " " + response.text).trim();
      status("Local transcript added.");
    } finally {
      button.disabled = false;
      button.textContent = "Record answer";
    }
    return;
  }
  recording = await startRecording();
  recording.button = button;
  button.textContent = "Stop and transcribe";
  status("Recording on this device…");
}
async function startRecording() {
  const stream = await navigator.mediaDevices.getUserMedia({audio: true});
  const context = new AudioContext();
  await context.audioWorklet.addModule("/assets/audio-worklet.js");
  const source = context.createMediaStreamSource(stream);
  const processor = new AudioWorkletNode(context, "pcm-recorder");
  const chunks = [];
  processor.port.onmessage = (event) => chunks.push(event.data);
  const mute = context.createGain();
  mute.gain.value = 0;
  source.connect(processor);
  processor.connect(mute);
  mute.connect(context.destination);
  return {stream, context, source, processor, chunks, sampleRate: context.sampleRate};
}
async function stopRecording() {
  const state = recording;
  recording = null;
  state.processor.disconnect();
  state.source.disconnect();
  state.stream.getTracks().forEach((track) => track.stop());
  await state.context.close();
  const size = state.chunks.reduce((count, chunk) => count + chunk.length, 0);
  const samples = new Float32Array(size);
  let offset = 0;
  state.chunks.forEach((chunk) => { samples.set(chunk, offset); offset += chunk.length; });
  return encodeWav(samples, state.sampleRate);
}
function encodeWav(samples, rate) {
  const buffer = new ArrayBuffer(44 + samples.length * 2);
  const view = new DataView(buffer);
  const word = (offset, value) => [...value].forEach((character, index) => view.setUint8(offset + index, character.charCodeAt(0)));
  word(0, "RIFF");
  view.setUint32(4, 36 + samples.length * 2, true);
  word(8, "WAVE");
  word(12, "fmt ");
  view.setUint32(16, 16, true);
  view.setUint16(20, 1, true);
  view.setUint16(22, 1, true);
  view.setUint32(24, rate, true);
  view.setUint32(28, rate * 2, true);
  view.setUint16(32, 2, true);
  view.setUint16(34, 16, true);
  word(36, "data");
  view.setUint32(40, samples.length * 2, true);
  samples.forEach((sample, index) => view.setInt16(44 + index * 2, Math.max(-1, Math.min(1, sample)) * 0x7fff, true));
  return new Blob([buffer], {type: "audio/wav"});
}
document.querySelector("#load").addEventListener("click", () => loadDeck().catch((error) => status(error.message)));
document.querySelector("#stats").addEventListener("click", () => showStats().catch((error) => status(error.message)));
boot().catch((error) => status(error.message));
