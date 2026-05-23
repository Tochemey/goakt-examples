// MIT License
//
// Copyright (c) 2022-2026 GoAkt Team
//
// Permission is hereby granted, free of charge, to any person obtaining a copy
// of this software and associated documentation files (the "Software"), to deal
// in the Software without restriction, including without limitation the rights
// to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
// copies of the Software, and to permit persons to whom the Software is
// furnished to do so, subject to the following conditions:
//
// The above copyright notice and this permission notice shall be included in all
// copies or substantial portions of the Software.
//
// THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
// IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
// FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
// AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
// LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
// OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
// SOFTWARE.

// goakt-pictograph browser client.
//
// Single .ts file kept deliberately small. Mirrors the wire payload
// declared in ../types.go — there's no codegen; the protocol is small
// enough to keep in sync by hand.

interface PlayerView { id: string; name: string; score: number; }
interface ScoreEntry { playerID: string; name: string; score: number; }
interface LeaderboardEntry { playerID: string; name: string; wins: number; }
interface ProfileView { playerID: string; name: string; gamesPlayed: number; wins: number; totalScore: number; }

// All event types prefixed `Msg` so they don't collide with DOM globals
// (ErrorEvent / MessageEvent / etc. — TS would otherwise try to merge them).
interface MsgJoined       { type: "joined"; room: string; playerID: string; profile: ProfileView; leaderboard: LeaderboardEntry[]; }
interface MsgState        { type: "state"; phase: string; round: number; maxRounds: number; timeLeft: number;
                            players: PlayerView[]; drawerID: string; wordMask: string; youAreDrawer: boolean; }
interface MsgStroke       { type: "stroke"; points: [number, number][]; color: number; width: number; }
interface MsgClear        { type: "clear"; }
interface MsgChat         { type: "chat"; from: string; text: string; }
interface MsgGuessed      { type: "guessed"; playerID: string; name: string; }
interface MsgScore        { type: "score"; playerID: string; delta: number; total: number; }
interface MsgWordChoices  { type: "wordChoices"; choices: string[]; }
interface MsgSecretWord   { type: "secretWord"; word: string; }
interface MsgRoundOver    { type: "roundOver"; word: string; scores: ScoreEntry[]; }
interface MsgGameOver     { type: "gameOver"; winnerID: string; winnerName: string; scores: ScoreEntry[]; leaderboard: LeaderboardEntry[]; }
interface MsgError        { type: "error"; message: string; }

type ServerMsg =
  MsgJoined | MsgState | MsgStroke | MsgClear | MsgChat | MsgGuessed
  | MsgScore | MsgWordChoices | MsgSecretWord | MsgRoundOver | MsgGameOver | MsgError;

// Palette must mirror the index ↔ color expected by the server (which
// just round-trips them). Index 0..7 used in StrokeEvent.color.
const PALETTE = [
  "#000000", "#e63946", "#f4a261", "#e9c46a",
  "#2a9d8f", "#264653", "#9d4edd", "#ffffff",
];

// ─── State ──────────────────────────────────────────────────────────────

const canvas       = document.getElementById("canvas") as HTMLCanvasElement;
const cctx         = canvas.getContext("2d")!;
const playersDiv   = document.getElementById("players")!;
const lbDiv        = document.getElementById("leaderboard")!;
const chatDiv      = document.getElementById("chat")!;
const chatInput    = document.getElementById("chatInput") as HTMLInputElement;
const bannerEl     = document.getElementById("banner")!;
const roundInfoEl  = document.getElementById("roundInfo")!;
const roundDotsEl  = document.getElementById("roundDots")!;
const timerEl      = document.getElementById("timer")!;
const progressFill = document.getElementById("progressFill")!;
const wordHint     = document.getElementById("wordHint")!;
const roomCodeEl   = document.getElementById("roomCode")!;
const toolbarEl    = document.getElementById("toolbar")!;
const overlay      = document.getElementById("overlayLayer")!;
const widthRange   = document.getElementById("widthRange") as HTMLInputElement;
const clearBtn     = document.getElementById("clearBtn") as HTMLButtonElement;
const swatches     = Array.from(toolbarEl.querySelectorAll(".swatch")) as HTMLElement[];
const roleLabel    = document.getElementById("role")!;
const toastsEl     = document.getElementById("toasts")!;

let myPlayerID = "";
let isDrawer   = false;
let secretWord = "";
let curColor   = 0;
let curWidth   = parseInt(widthRange.value, 10);
let strokeBuf: [number, number][] = [];
let drawing    = false;

// Round-level state tracked so we can detect transitions and drive
// the UI accordingly (round-start toast, "you guessed!" lock-out on
// the chat input, etc.).
let prevPhase    = "";
let prevDrawerID = "";
let prevRound    = 0;
let iHaveGuessed = false;

// Phase durations the server uses (mirrored from types.go). We use
// these to scale the progress bar — the wire protocol only sends the
// remaining seconds, so we need the max to compute a percentage.
const PHASE_MAX_SECS: Record<string, number> = {
  choosing:  15,
  drawing:   80,
  roundOver:  5,
  gameOver:  20,
};

// URL params: name + id are both per-tab (sessionStorage) so opening
// multiple tabs in the same browser gives each a distinct player
// identity and prompts for a fresh name. Reloading within a tab keeps
// both values because sessionStorage survives F5. ?name= / ?id=
// query parameters always win.
const params = new URLSearchParams(location.search);
let displayName = params.get("name") || sessionStorage.getItem("pictograph.name") || "";
let userID      = params.get("id")   || sessionStorage.getItem("pictograph.id")   || "";
const room      = (params.get("room") || "").toUpperCase();

if (!displayName) {
  displayName = prompt("Pick a display name for this tab") || "Player";
}
sessionStorage.setItem("pictograph.name", displayName);
if (!userID) {
  userID = cryptoRandom();
  sessionStorage.setItem("pictograph.id", userID);
}

function cryptoRandom(): string {
  const a = new Uint8Array(8);
  crypto.getRandomValues(a);
  return Array.from(a, b => b.toString(16).padStart(2, "0")).join("");
}

// ─── WebSocket ──────────────────────────────────────────────────────────

const wsURL = new URL("ws", location.href);
wsURL.protocol = location.protocol === "https:" ? "wss:" : "ws:";
wsURL.searchParams.set("name", displayName);
wsURL.searchParams.set("id", userID);
if (room) wsURL.searchParams.set("room", room);

const ws = new WebSocket(wsURL.toString());
ws.addEventListener("open",    () => setBanner("waiting", "connecting…"));
ws.addEventListener("close",   () => setBanner("waiting", "disconnected"));
ws.addEventListener("error",   () => setBanner("waiting", "connection error"));
ws.addEventListener("message", (e) => onMessage(JSON.parse(e.data) as ServerMsg));

function send(payload: object) {
  if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify(payload));
}

// ─── Inbound dispatch ───────────────────────────────────────────────────

function onMessage(ev: ServerMsg) {
  switch (ev.type) {
    case "joined":      onJoined(ev); break;
    case "state":       onState(ev);  break;
    case "stroke":      drawStroke(ev.points, ev.color, ev.width); break;
    case "clear":       clearCanvas(); break;
    case "chat":        addChat(ev.from, ev.text, "msg"); break;
    case "guessed":
      addChat("✓", `${ev.name} guessed the word!`, "good system");
      if (ev.playerID === myPlayerID) {
        iHaveGuessed = true;
        toast("✓ You guessed the word!", "good");
        chatInput.disabled = true;
        chatInput.placeholder = "you guessed!";
      } else {
        toast(`✓ ${ev.name} guessed the word`, "good");
      }
      break;
    case "score":
      if (ev.playerID === myPlayerID) {
        toast(`+${ev.delta} points`, "score");
      }
      break;
    case "wordChoices": showWordChoices(ev.choices); break;
    case "secretWord":  secretWord = ev.word; wordHint.textContent = ev.word; break;
    case "roundOver":   showRoundOver(ev); break;
    case "gameOver":    showGameOver(ev); break;
    case "error":       addChat("!", ev.message, "system bad"); break;
  }
}

function onJoined(ev: MsgJoined) {
  myPlayerID = ev.playerID;
  roomCodeEl.textContent = ev.room;
  // Persist the room into the URL so a reload returns to the same one,
  // and so the link is shareable.
  const u = new URL(location.href);
  u.searchParams.set("room", ev.room);
  history.replaceState(null, "", u.toString());
  renderLeaderboard(ev.leaderboard);
  addChat("·", `joined room ${ev.room} as ${displayName}`, "system");
}

function onState(ev: MsgState) {
  isDrawer = ev.youAreDrawer;

  // Phase transition: clear any stale overlay (e.g. the gameOver
  // winner reveal after Play Again jumps us back into choosing).
  // Phase-specific overlays (wordChoices for the drawer; the new
  // gameOver reveal) re-show themselves via their own events that
  // arrive after the StateEvent.
  if (ev.phase !== prevPhase) {
    clearOverlay();
    prevPhase = ev.phase;
  }

  // Detect round transitions: reset "I've guessed" lockout and toast
  // the new drawer for everyone except the drawer themselves (the
  // drawer already sees a prominent word-choice overlay).
  if (ev.round !== prevRound || ev.drawerID !== prevDrawerID) {
    iHaveGuessed = false;
    const drawerName = nameOf(ev.drawerID, ev);
    if (ev.round > 0 && drawerName && ev.drawerID !== myPlayerID) {
      toast(`🎨 ${drawerName} is drawing — round ${ev.round}/${ev.maxRounds}`, "info");
    }
  }
  prevRound    = ev.round;
  prevDrawerID = ev.drawerID;

  // Word display: drawer sees secret, others see mask of underscores
  // (only meaningful during the drawing phase — clear in others).
  if (ev.phase === "drawing") {
    wordHint.textContent = ev.youAreDrawer ? secretWord : ev.wordMask;
  } else {
    wordHint.textContent = "";
  }

  // Round indicator + dots
  roundInfoEl.textContent = ev.round > 0 ? `round ${ev.round}/${ev.maxRounds}` : "round —";
  renderRoundDots(ev.round, ev.maxRounds);

  // Timer + progress bar (colour-coded as the round gets tight)
  updateTimerAndProgress(ev.phase, ev.timeLeft);

  // Phase-aware banner — replaces the old phase label
  updateBanner(ev);

  // Toolbar / canvas mode
  const canDraw = isDrawer && ev.phase === "drawing";
  toolbarEl.classList.toggle("disabled", !canDraw);
  canvas.classList.toggle("spectator", !canDraw);
  roleLabel.textContent = canDraw ? "you are drawing" : (iHaveGuessed ? "you guessed!" : "guessing…");

  // Chat-input mode during drawing: drawer can't guess; once you've
  // guessed correctly, the input locks until the next round.
  if (ev.phase === "drawing") {
    chatInput.disabled = isDrawer || iHaveGuessed;
    chatInput.placeholder = isDrawer
      ? "you're the drawer — can't guess"
      : (iHaveGuessed ? "you guessed!" : "Type your guess and press Enter…");
  } else {
    chatInput.disabled = false;
    chatInput.placeholder = "Type a message…";
  }

  renderPlayers(ev);
}

function nameOf(id: string, ev: MsgState): string {
  const p = ev.players.find((p) => p.id === id);
  return p ? p.name : "";
}

function renderPlayers(ev: MsgState) {
  playersDiv.innerHTML = "";
  for (const p of ev.players) {
    const row = document.createElement("div");
    row.className = "row";
    if (p.id === ev.drawerID) row.classList.add("drawer");
    if (p.id === myPlayerID)  row.classList.add("you");
    row.innerHTML = `<span class="name">${escapeHTML(p.name)}</span><span class="score">${p.score}</span>`;
    playersDiv.appendChild(row);
  }
}

// updateTimerAndProgress refreshes both the textual timer and the
// width-driven progress bar at the foot of the header. Below 10 s it
// switches to warn (orange), below 5 s to crit (red) — gives a clear
// visual cue without needing the player to read numbers.
function updateTimerAndProgress(phase: string, timeLeft: number) {
  const max = PHASE_MAX_SECS[phase] ?? 0;
  if (max === 0 || timeLeft < 0) {
    timerEl.textContent = "—";
    timerEl.className = "timer";
    progressFill.style.width = "0%";
    progressFill.className = "fill";
    return;
  }
  timerEl.textContent = `${timeLeft}s`;
  const pct = Math.max(0, Math.min(100, (timeLeft / max) * 100));
  progressFill.style.width = `${pct}%`;

  let kind = "";
  if (phase === "drawing") {
    if (timeLeft <= 5)       kind = "crit";
    else if (timeLeft <= 10) kind = "warn";
  }
  timerEl.className = "timer" + (kind ? " " + kind : "");
  progressFill.className = "fill" + (kind ? " " + kind : "");
}

// renderRoundDots draws one dot per round (done / current / upcoming)
// so players can see at a glance how far through the game they are.
function renderRoundDots(round: number, maxRounds: number) {
  roundDotsEl.innerHTML = "";
  if (maxRounds <= 0) return;
  for (let i = 1; i <= maxRounds; i++) {
    const dot = document.createElement("span");
    dot.className = "dot";
    if (i < round)       dot.classList.add("done");
    else if (i === round) dot.classList.add("current");
    roundDotsEl.appendChild(dot);
  }
}

// updateBanner sets the full-width strip below the header so the
// player's role and the current expectation are unmissable — much
// more visible than the old small "phase" label.
function updateBanner(ev: MsgState) {
  const drawerName = nameOf(ev.drawerID, ev);

  switch (ev.phase) {
    case "waiting":
      setBanner("waiting", "⏳ Waiting for at least 2 players…");
      return;
    case "choosing":
      if (ev.youAreDrawer) {
        setBanner("choosing", "🎨 Pick a word from the choices to start drawing!");
      } else {
        setBanner("choosing", `⏳ ${drawerName} is picking a word…`);
      }
      return;
    case "drawing":
      if (ev.youAreDrawer) {
        setBanner("drawing", `✏️ You're drawing — others are guessing!`);
      } else if (iHaveGuessed) {
        setBanner("drawing", `✓ You guessed it! Waiting for the round to end…`);
      } else {
        setBanner("drawing", `💭 Guess the word ${drawerName} is drawing!`);
      }
      return;
    case "roundOver":
      setBanner("waiting", `📝 Round over — next round starts soon…`);
      return;
    case "gameOver":
      setBanner("gameover", `🏆 Game over!`);
      return;
  }
}

function setBanner(kind: "waiting" | "choosing" | "drawing" | "gameover", text: string) {
  bannerEl.textContent = text;
  bannerEl.className = "banner " + kind;
}

// toast shows a transient notification top-right. The CSS handles the
// fade-out animation; we just remove the element after it finishes.
function toast(text: string, kind: "info" | "good" | "warn" | "score" = "info") {
  const el = document.createElement("div");
  el.className = "toast" + (kind === "info" ? "" : " " + kind);
  el.textContent = text;
  toastsEl.appendChild(el);
  setTimeout(() => el.remove(), 3100);
}

function renderLeaderboard(entries: LeaderboardEntry[] | null | undefined) {
  lbDiv.innerHTML = "";
  if (!entries || entries.length === 0) {
    const row = document.createElement("div");
    row.className = "row";
    row.innerHTML = `<span style="color:var(--muted)">no wins yet</span>`;
    lbDiv.appendChild(row);
    return;
  }
  for (const e of entries) {
    const row = document.createElement("div");
    row.className = "row";
    row.innerHTML = `<span class="name">${escapeHTML(e.name || e.playerID.substring(0, 6))}</span>
                     <span class="wins">${e.wins}🏆</span>`;
    lbDiv.appendChild(row);
  }
}

function addChat(from: string, text: string, cls = "msg") {
  const row = document.createElement("div");
  row.className = `msg ${cls}`;
  row.innerHTML = `<span class="from">${escapeHTML(from)}</span>${escapeHTML(text)}`;
  chatDiv.appendChild(row);
  chatDiv.scrollTop = chatDiv.scrollHeight;
}

function escapeHTML(s: string): string {
  return s.replace(/[&<>"']/g, (c) => ({"&":"&amp;","<":"&lt;",">":"&gt;",'"':"&quot;","'":"&#39;"}[c]!));
}

// ─── Canvas ─────────────────────────────────────────────────────────────

function clearCanvas() {
  cctx.fillStyle = "#ffffff";
  cctx.fillRect(0, 0, canvas.width, canvas.height);
}

function drawStroke(points: [number, number][], colorIdx: number, width: number) {
  if (!points || points.length === 0) return;
  cctx.strokeStyle = PALETTE[colorIdx] || "#000000";
  cctx.lineWidth = width;
  cctx.lineCap = "round";
  cctx.lineJoin = "round";
  cctx.beginPath();
  const [x0, y0] = points[0];
  cctx.moveTo(x0 * canvas.width, y0 * canvas.height);
  for (let i = 1; i < points.length; i++) {
    const [x, y] = points[i];
    cctx.lineTo(x * canvas.width, y * canvas.height);
  }
  cctx.stroke();
}

clearCanvas();

canvas.addEventListener("pointerdown", (e) => {
  if (!isDrawer) return;
  drawing = true;
  strokeBuf = [normalizePoint(e)];
  canvas.setPointerCapture(e.pointerId);
});

canvas.addEventListener("pointermove", (e) => {
  if (!drawing) return;
  strokeBuf.push(normalizePoint(e));
  // Flush small batches so the network sees ~30 Hz updates rather than
  // one fat send per stroke. Server fans these out over the topic.
  if (strokeBuf.length >= 6) {
    flushStroke();
  }
});

canvas.addEventListener("pointerup",     () => endStroke());
canvas.addEventListener("pointercancel", () => endStroke());
canvas.addEventListener("pointerleave",  () => endStroke());

function endStroke() {
  if (!drawing) return;
  drawing = false;
  if (strokeBuf.length > 0) flushStroke();
}

function flushStroke() {
  if (strokeBuf.length < 2) {
    strokeBuf = [];
    return;
  }
  send({ type: "stroke", points: strokeBuf, color: curColor, width: curWidth });
  // Render locally too — the server echoes the stroke back via pub/sub
  // but we draw immediately for snappier feedback.
  drawStroke(strokeBuf, curColor, curWidth);
  // Keep the last point so the next batch joins continuously.
  strokeBuf = [strokeBuf[strokeBuf.length - 1]];
}

function normalizePoint(e: PointerEvent): [number, number] {
  const rect = canvas.getBoundingClientRect();
  const x = (e.clientX - rect.left) / rect.width;
  const y = (e.clientY - rect.top)  / rect.height;
  return [Math.max(0, Math.min(1, x)), Math.max(0, Math.min(1, y))];
}

clearBtn.addEventListener("click", () => {
  if (!isDrawer) return;
  send({ type: "clear" });
});

widthRange.addEventListener("input", () => { curWidth = parseInt(widthRange.value, 10); });

swatches.forEach((sw, i) => {
  if (i === 0) sw.classList.add("active");
  sw.addEventListener("click", () => {
    swatches.forEach((s) => s.classList.remove("active"));
    sw.classList.add("active");
    curColor = i;
  });
});

// ─── Chat / guess input ─────────────────────────────────────────────────

chatInput.addEventListener("keydown", (e) => {
  if (e.key !== "Enter") return;
  const text = chatInput.value.trim();
  if (!text) return;
  send({ type: "guess", text });
  chatInput.value = "";
});

// ─── Word-choice overlay (drawer only, during choosing) ─────────────────

function showWordChoices(choices: string[]) {
  clearOverlay();
  const div = document.createElement("div");
  div.className = "overlay";
  div.innerHTML = `<h3>Pick a word to draw</h3>`;
  for (const w of choices) {
    const b = document.createElement("button");
    b.textContent = w;
    b.onclick = () => {
      send({ type: "pickWord", word: w });
      clearOverlay();
    };
    div.appendChild(b);
  }
  overlay.appendChild(div);
}

function showRoundOver(ev: MsgRoundOver) {
  clearOverlay();
  const div = document.createElement("div");
  div.className = "overlay";
  const rows = ev.scores
    .slice()
    .sort((a, b) => b.score - a.score)
    .map((s) => `<div>${escapeHTML(s.name)}: ${s.score}</div>`)
    .join("");
  div.innerHTML = `<h3>The word was <em>${escapeHTML(ev.word)}</em></h3>${rows}`;
  overlay.appendChild(div);
  setTimeout(clearOverlay, 4500);
}

function showGameOver(ev: MsgGameOver) {
  clearOverlay();
  const div = document.createElement("div");
  div.className = "overlay";
  const rows = ev.scores
    .slice()
    .sort((a, b) => b.score - a.score)
    .map((s) => `<div>${escapeHTML(s.name)}: ${s.score}</div>`)
    .join("");
  div.innerHTML = `<h3>🏆 ${escapeHTML(ev.winnerName || "(nobody)")} wins!</h3>${rows}`;

  // Play Again button. First click sends the restart message; we
  // disable the button locally so the user gets immediate feedback.
  // When the server processes the restart, a StateEvent(choosing) +
  // ChatEvent("X started a new game") arrives and the overlay clears
  // via the phase-change path in onState.
  const btn = document.createElement("button");
  btn.textContent = "▶ Play Again";
  btn.onclick = () => {
    send({ type: "restart" });
    btn.disabled = true;
    btn.textContent = "restarting…";
  };
  const br = document.createElement("div");
  br.style.marginTop = "12px";
  br.appendChild(btn);
  div.appendChild(br);

  overlay.appendChild(div);
  if (ev.leaderboard) renderLeaderboard(ev.leaderboard);
}

function clearOverlay() {
  overlay.innerHTML = "";
}
