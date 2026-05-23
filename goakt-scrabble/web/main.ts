// goakt-scrabble browser client
// Vanilla TypeScript. All wire shapes match goakt-scrabble/types.go.

interface PlayerView { id: string; name: string; score: number; rackSize: number; bot: boolean; }
interface FormedWord { word: string; score: number; }
interface PlacementWire { row: number; col: number; letter: string; blank?: boolean; }
interface ScoreEntry { playerID: string; name: string; score: number; }
interface LeaderboardEntry { playerID: string; name: string; wins: number; }

interface JoinedMsg { type: "joined"; room: string; language: string; playerID: string; owner: boolean; profile: any; leaderboard: LeaderboardEntry[] | null; }
interface StateMsg { type: "state"; phase: string; board: string[][]; yourRack: string[]; players: PlayerView[]; currentID: string; ownerID: string; bagRemaining: number; timerMs: number; }
interface MoveMsg { type: "move"; playerID: string; name: string; placements: PlacementWire[]; words: FormedWord[]; score: number; newTotal: number; bingo: boolean; }
interface ChatMsg { type: "chat"; from: string; text: string; }
interface ErrorMsg { type: "error"; message: string; }
interface GameOverMsg { type: "gameOver"; winnerID: string; winnerName: string; scores: ScoreEntry[]; leaderboard: LeaderboardEntry[] | null; }
type Msg = JoinedMsg | StateMsg | MoveMsg | ChatMsg | ErrorMsg | GameOverMsg;

interface Pending { rackIdx: number; row: number; col: number; letter: string; blank: boolean; }

interface LogMove { kind: "move"; playerID: string; name: string; words: FormedWord[]; score: number; bingo: boolean; }
interface LogEvent { kind: "event"; text: string; error?: boolean; }
type LogEntry = LogMove | LogEvent;

type DragSource =
  | { kind: "rack"; idx: number }
  | { kind: "board"; row: number; col: number };

interface Drag {
  source: DragSource;
  sourceEl: HTMLElement | SVGElement;
  letter: string;
  blank: boolean;
  startX: number;
  startY: number;
  pointerId: number;
  ghost: HTMLElement | null;
  active: boolean;
  hintEl: SVGRectElement | null;
  rackHint: boolean;
}

const BOARD_SIZE = 15;
const CENTER = 7;
const LAST_MOVE_HIGHLIGHT_MS = 6000;
const DRAG_THRESHOLD = 6; // px of pointer travel before a press becomes a drag

// Internal SVG units — actual rendered size is controlled by CSS.
const CELL_UNIT = 40;
const BOARD_UNIT = CELL_UNIT * BOARD_SIZE;

const POINT_VALUES_EN: Record<string, number> = {
  A: 1, B: 3, C: 3, D: 2, E: 1, F: 4, G: 2, H: 4, I: 1, J: 8, K: 5, L: 1, M: 3,
  N: 1, O: 1, P: 3, Q: 10, R: 1, S: 1, T: 1, U: 1, V: 4, W: 4, X: 8, Y: 4, Z: 10,
};

const DISTRIBUTION_EN: Record<string, number> = {
  A: 9, B: 2, C: 2, D: 4, E: 12, F: 2, G: 3, H: 2, I: 9, J: 1, K: 1, L: 4, M: 2,
  N: 6, O: 8, P: 2, Q: 1, R: 6, S: 4, T: 6, U: 4, V: 2, W: 2, X: 1, Y: 2, Z: 1,
  "?": 2,
};

const state = {
  ws: null as WebSocket | null,
  playerID: "",
  name: "",
  language: "en",
  owner: false,
  roomCode: "",
  phase: "waiting",
  board: [] as string[][],
  yourRack: [] as string[],
  players: [] as PlayerView[],
  currentID: "",
  ownerID: "",
  bagRemaining: 100,
  pending: [] as Pending[],
  exchangeMode: false,
  exchangeSelection: new Set<number>(),
  selectedRackIdx: -1,
  turnDeadlineMs: 0,
  gameOverShown: false,
  leaderboard: [] as LeaderboardEntry[],
  log: [] as LogEntry[],
  lastMove: null as { placements: PlacementWire[]; expiresAt: number } | null,
};

let drag: Drag | null = null;
let suppressClickUntil = 0;

// ------------------------------------------------------------ premium squares

type Premium = "" | "DL" | "TL" | "DW" | "TW" | "C";

function premiumAt(row: number, col: number): Premium {
  if (row === CENTER && col === CENTER) return "C";
  let r = row, c = col;
  if (r > 7) r = 14 - r;
  if (c > 7) c = 14 - c;
  if (r > c) [r, c] = [c, r];
  const key = `${r},${c}`;
  switch (key) {
    case "0,0": case "0,7": return "TW";
    case "1,1": case "2,2": case "3,3": case "4,4": case "7,7": return "DW";
    case "1,5": case "5,5": return "TL";
    case "0,3": case "2,6": case "3,7": case "6,6": return "DL";
  }
  return "";
}

function premiumColor(p: Premium): string {
  switch (p) {
    case "DL": return "var(--dl)";
    case "TL": return "var(--tl)";
    case "DW": return "var(--dw)";
    case "TW": return "var(--tw)";
    case "C":  return "var(--center)";
    default:   return "var(--cell)";
  }
}

function premiumLabel(p: Premium): string {
  switch (p) {
    case "DL": return "DL";
    case "TL": return "TL";
    case "DW": return "DW";
    case "TW": return "TW";
    case "C":  return "★";
    default:   return "";
  }
}

// ------------------------------------------------------------ DOM helpers

function $<T extends HTMLElement>(id: string): T { return document.getElementById(id) as T; }

function el<K extends keyof HTMLElementTagNameMap>(tag: K, attrs: Record<string, string> = {}, ...children: (Node | string)[]): HTMLElementTagNameMap[K] {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") node.className = v;
    else if (k.startsWith("data-")) node.setAttribute(k, v);
    else (node as any)[k] = v;
  }
  for (const c of children) node.append(c);
  return node;
}

function svg(tag: string, attrs: Record<string, string | number> = {}, ...children: (Node | string)[]): SVGElement {
  const node = document.createElementNS("http://www.w3.org/2000/svg", tag);
  for (const [k, v] of Object.entries(attrs)) node.setAttribute(k, String(v));
  for (const c of children) node.append(c);
  return node;
}

// ------------------------------------------------------------ tile helpers

function tilePoints(letter: string, blank: boolean): number {
  if (blank) return 0;
  return POINT_VALUES_EN[letter.toUpperCase()] ?? 0;
}

function parseTileWire(raw: string): { letter: string; blank: boolean; empty: boolean } {
  if (!raw) return { letter: "", blank: false, empty: true };
  if (raw === "?") return { letter: "?", blank: true, empty: false };
  if (raw.startsWith("*")) return { letter: raw.slice(1).toUpperCase(), blank: true, empty: false };
  return { letter: raw.toUpperCase(), blank: false, empty: false };
}

// ------------------------------------------------------------ unseen-tile breakdown

// Tiles a serious player can "see": on the board + in their own rack.
// Everything else (bag + opponents' racks) is unknown — and that's exactly
// what we surface, since that's what real Scrabble players track.
function unseenBreakdown(): Record<string, number> {
  const counts: Record<string, number> = { ...DISTRIBUTION_EN };

  for (let r = 0; r < BOARD_SIZE; r++) {
    for (let c = 0; c < BOARD_SIZE; c++) {
      const raw = state.board[r]?.[c];
      if (!raw) continue;
      const t = parseTileWire(raw);
      if (t.empty) continue;
      const key = t.blank ? "?" : t.letter;
      counts[key] = (counts[key] ?? 0) - 1;
    }
  }

  for (const raw of state.yourRack) {
    const t = parseTileWire(raw);
    if (t.empty) continue;
    const key = t.blank ? "?" : t.letter;
    counts[key] = (counts[key] ?? 0) - 1;
  }

  return counts;
}

// ------------------------------------------------------------ render

function render() {
  renderHeader();
  renderPlay();
  renderPlayers();
  renderLobbyActions();
  renderBag();
  renderMovesLog();
}

function renderHeader() {
  $("roomCode").textContent = state.roomCode || "—";
  $("langCode").textContent = state.language || "en";
  $("bagBadge").textContent = String(state.bagRemaining);
}

function renderPlay() {
  const play = $("play");
  play.innerHTML = "";

  if (state.phase === "waiting") {
    play.append(renderLobbyCard());
    return;
  }

  play.append(renderBoardFrame(), renderRack(), renderControls());

  if (state.phase === "gameOver" && !state.gameOverShown) {
    state.gameOverShown = true;
    showGameOverOverlay();
  }
}

function renderLobbyCard(): HTMLElement {
  const card = el("div", { class: "lobby-card" });
  card.append(el("h2", {}, "Room " + state.roomCode));
  if (state.owner) {
    card.append(el("p", {}, `You're the host. ${state.players.length < 2 ? "Add a bot or invite a friend (share the room code) to start." : "Add more bots or press Start to begin."}`));
  } else {
    card.append(el("p", {}, "Waiting for the host to start the game…"));
  }
  return card;
}

function renderBoardFrame(): HTMLElement {
  const frame = el("div", { class: "board-frame" });
  frame.append(renderBoard());
  return frame;
}

function renderBoard(): SVGElement {
  const root = svg("svg", { viewBox: `0 0 ${BOARD_UNIT} ${BOARD_UNIT}`, preserveAspectRatio: "xMidYMid meet" });

  type Cell = { letter: string; blank: boolean; pending: boolean; recent: boolean };
  const view: Cell[][] = [];
  const recent = new Set<string>();
  if (state.lastMove && Date.now() < state.lastMove.expiresAt) {
    for (const p of state.lastMove.placements) recent.add(`${p.row},${p.col}`);
  }

  for (let r = 0; r < BOARD_SIZE; r++) {
    view[r] = [];
    for (let c = 0; c < BOARD_SIZE; c++) {
      const raw = state.board[r]?.[c] ?? "";
      const parsed = parseTileWire(raw);
      view[r][c] = {
        letter: parsed.letter,
        blank: parsed.blank,
        pending: false,
        recent: recent.has(`${r},${c}`),
      };
    }
  }
  for (const p of state.pending) {
    view[p.row][p.col] = { letter: p.letter, blank: p.blank, pending: true, recent: false };
  }

  for (let r = 0; r < BOARD_SIZE; r++) {
    for (let c = 0; c < BOARD_SIZE; c++) {
      const x = c * CELL_UNIT;
      const y = r * CELL_UNIT;
      const cell = view[r][c];
      const p = premiumAt(r, c);
      const isFilled = !!cell.letter && cell.letter !== "?";
      const fill = isFilled ? "var(--cell)" : premiumColor(p);

      const rect = svg("rect", {
        x: x + 0.5, y: y + 0.5,
        width: CELL_UNIT - 1, height: CELL_UNIT - 1,
        rx: 2, fill,
        stroke: "var(--cell-line)", "stroke-width": 0.5,
        "data-row": String(r), "data-col": String(c),
      }) as SVGRectElement;
      rect.style.cursor = "pointer";
      rect.addEventListener("click", () => onBoardClick(r, c));
      root.append(rect);

      if (!isFilled) {
        const label = premiumLabel(p);
        if (label) {
          const isStar = label === "★";
          root.append(svg("text", {
            x: x + CELL_UNIT / 2, y: y + CELL_UNIT / 2 + (isStar ? 7 : 4),
            "text-anchor": "middle",
            "font-size": isStar ? 22 : 11,
            "font-weight": isStar ? 400 : 800,
            "letter-spacing": isStar ? 0 : 0.5,
            fill: "var(--premium-ink)",
            opacity: 0.7,
            "pointer-events": "none",
          }, label));
        }
      } else {
        root.append(renderTile(x, y, cell.letter, cell.blank, cell.pending, cell.recent, r, c));
      }
    }
  }

  return root;
}

function renderTile(x: number, y: number, letter: string, blank: boolean, pending: boolean, recent: boolean, row: number, col: number): SVGElement {
  const inset = 2;
  const w = CELL_UNIT - inset * 2;
  const fill = pending ? "var(--tile-pending)" : "var(--tile)";

  const group = svg("g", {
    "pointer-events": pending ? "auto" : "none",
    style: pending ? "cursor: pointer" : "",
  });

  // shadow plate underneath gives the slight 3D feel
  group.append(svg("rect", {
    x: x + inset, y: y + inset + 0.5,
    width: w, height: w - 1,
    rx: 3, fill: "var(--tile-shadow)",
  }));
  group.append(svg("rect", {
    x: x + inset, y: y + inset,
    width: w, height: w - 2,
    rx: 3, fill,
    stroke: pending ? "var(--tile-pending-stroke)" : (recent ? "var(--tile-recent-stroke)" : "none"),
    "stroke-width": pending ? 2.4 : (recent ? 1.6 : 0),
  }));
  group.append(svg("text", {
    x: x + CELL_UNIT / 2, y: y + CELL_UNIT / 2 + 7,
    "text-anchor": "middle",
    "font-size": 22, "font-weight": 700,
    fill: blank ? "#6b5640" : "var(--tile-ink)",
    "font-style": blank ? "italic" : "normal",
    "pointer-events": "none",
  }, letter));

  const pts = tilePoints(letter, blank);
  if (pts > 0) {
    group.append(svg("text", {
      x: x + CELL_UNIT - 5, y: y + CELL_UNIT - 5,
      "text-anchor": "end",
      "font-size": 10, "font-weight": 700,
      fill: "var(--tile-ink)", opacity: 0.7,
      "pointer-events": "none",
    }, String(pts)));
  }

  if (pending) {
    group.addEventListener("click", () => onPendingClick(row, col));
    group.addEventListener("pointerdown", (e) => {
      startDrag(e, { kind: "board", row, col }, letter, blank);
    });
  }

  return group;
}

function renderRack(): HTMLElement {
  const rack = el("div", { class: "rack" });
  for (let i = 0; i < state.yourRack.length; i++) {
    const raw = state.yourRack[i];
    const parsed = parseTileWire(raw);
    const spent = state.pending.some(p => p.rackIdx === i);

    let cls = "rack-tile";
    if (parsed.blank) cls += " blank";
    if (state.selectedRackIdx === i) cls += " selected";
    if (state.exchangeSelection.has(i)) cls += " exchange-pick";
    if (spent) cls += " spent";

    const tile = el("div", { class: cls }, parsed.letter || "?");
    if (parsed.letter && !parsed.blank) {
      tile.append(el("span", { class: "pts" }, String(tilePoints(parsed.letter, false))));
    }
    tile.addEventListener("click", () => onRackClick(i));
    if (!spent) {
      tile.addEventListener("pointerdown", (e) => {
        startDrag(e, { kind: "rack", idx: i }, parsed.letter || "?", parsed.blank);
      });
    }
    rack.append(tile);
  }
  return rack;
}

function renderControls(): HTMLElement {
  const wrap = el("div", { class: "controls" });
  const myTurn = state.currentID === state.playerID && state.phase === "playing";
  const hasPending = state.pending.length > 0;
  const isPaused = state.phase === "paused";

  if (state.exchangeMode && !isPaused) {
    const picked = state.exchangeSelection.size;
    const label = picked === 0 ? "Confirm Exchange" : `Confirm Exchange (${picked})`;
    const confirm = el("button", { class: "primary" }, label);
    if (picked === 0) confirm.setAttribute("disabled", "");
    confirm.addEventListener("click", confirmExchange);
    const cancel = el("button", {}, "Cancel");
    cancel.addEventListener("click", () => { state.exchangeMode = false; state.exchangeSelection.clear(); render(); });
    const hint = picked === 0
      ? "Click rack tiles to mark them for swap; click again to unmark."
      : `${picked} marked. Click to add or remove; Confirm to draw replacements.`;
    wrap.append(confirm, cancel, el("span", { class: "hint" }, hint));
    return wrap;
  }

  if (isPaused) {
    wrap.append(el("span", { class: "paused-badge" }, "PAUSED"));
    const resume = el("button", { class: "primary" }, "▶ Resume");
    resume.addEventListener("click", () => send({ type: "resume" }));
    wrap.append(resume);
    return wrap;
  }

  if (myTurn) {
    wrap.append(el("span", { class: "turn-badge" }, "YOUR TURN"));
  } else if (state.phase === "playing") {
    const current = state.players.find(p => p.id === state.currentID);
    wrap.append(el("span", { class: "waiting-badge" }, current ? `${current.name} is thinking…` : "Waiting…"));
  }

  const submit = el("button", { class: "primary" }, "Submit");
  if (!myTurn || !hasPending) submit.setAttribute("disabled", "");
  submit.addEventListener("click", submitMove);

  const recall = el("button", {}, "Recall");
  if (!hasPending) recall.setAttribute("disabled", "");
  recall.addEventListener("click", () => { state.pending = []; state.selectedRackIdx = -1; render(); });

  const shuffle = el("button", {}, "Shuffle");
  shuffle.addEventListener("click", () => {
    for (let i = state.yourRack.length - 1; i > 0; i--) {
      const j = Math.floor(Math.random() * (i + 1));
      [state.yourRack[i], state.yourRack[j]] = [state.yourRack[j], state.yourRack[i]];
    }
    state.pending = [];
    state.selectedRackIdx = -1;
    render();
  });

  const exchange = el("button", {}, "Exchange");
  // Scrabble rule: bag must have at least 7 tiles for an exchange to be legal.
  const bagTooLow = state.bagRemaining < 7;
  if (!myTurn || hasPending || bagTooLow) exchange.setAttribute("disabled", "");
  if (bagTooLow) exchange.title = `Bag has ${state.bagRemaining} tiles; exchange needs at least 7.`;
  exchange.addEventListener("click", () => { state.exchangeMode = true; state.exchangeSelection.clear(); render(); });

  const pass = el("button", {}, "Pass");
  if (!myTurn) pass.setAttribute("disabled", "");
  pass.addEventListener("click", () => { send({ type: "pass" }); });

  const pause = el("button", {}, "⏸ Pause");
  if (state.phase !== "playing") pause.setAttribute("disabled", "");
  pause.addEventListener("click", () => send({ type: "pause" }));

  wrap.append(submit, recall, shuffle, exchange, pass, pause);
  return wrap;
}

function renderPlayers() {
  const list = $("playerList");
  list.innerHTML = "";
  for (let i = 0; i < state.players.length; i++) {
    const p = state.players[i];
    const li = el("li", { class: p.id === state.currentID ? "current" : "" });
    li.append(el("span", { class: "name" }, p.name));
    if (p.bot) li.append(el("span", { class: "bot-tag" }, "BOT"));
    if (state.phase === "playing" || state.phase === "gameOver") {
      li.append(el("span", { class: "rack-count" }, `· ${p.rackSize} tiles`));
    }
    li.append(el("span", { class: "score" }, String(p.score)));
    if (state.phase === "waiting" && state.owner && p.bot) {
      const seat = i;
      const remove = el("button", {}, "✕");
      remove.addEventListener("click", () => send({ type: "removeBot", seat }));
      li.append(remove);
    }
    list.append(li);
  }
}

function renderLobbyActions() {
  const wrap = $("lobbyActions");
  wrap.innerHTML = "";
  if (state.phase !== "waiting" || !state.owner) return;
  const addBot = el("button", {}, "+ Add Bot");
  if (state.players.length >= 4) addBot.setAttribute("disabled", "");
  addBot.addEventListener("click", () => send({ type: "addBot" }));
  const start = el("button", { class: "primary" }, "Start Game");
  if (state.players.length < 2) start.setAttribute("disabled", "");
  start.addEventListener("click", () => send({ type: "start" }));
  wrap.append(addBot, start);
}

function renderBag() {
  const section = $("bagSection");
  if (state.phase === "waiting") {
    section.style.display = "none";
    return;
  }
  section.style.display = "";

  const wrap = $("bagLetters");
  wrap.innerHTML = "";
  const counts = unseenBreakdown();
  const letters = "ABCDEFGHIJKLMNOPQRSTUVWXYZ".split("");
  letters.push("?");

  for (const ch of letters) {
    const n = counts[ch] ?? 0;
    const cls = "bag-letter " + (n > 0 ? "has" : "gone");
    const cell = el("div", { class: cls });
    cell.append(el("span", { class: "ch" }, ch));
    cell.append(el("span", { class: "n" }, String(Math.max(0, n))));
    wrap.append(cell);
  }
}

function renderMovesLog() {
  const log = $("movesLog");
  log.innerHTML = "";

  if (state.log.length === 0) {
    log.append(el("li", { class: "moves-log-empty" }, "No moves played yet."));
    return;
  }

  // Newest first.
  for (let i = state.log.length - 1; i >= 0; i--) {
    const entry = state.log[i];
    const li = el("li", {});
    if (entry.kind === "move") {
      const words = el("div", { class: "words" });
      words.append(el("div", { class: "who" }, entry.name));
      const wordText = entry.words.map(w => `${w.word} (${w.score})`).join(" · ");
      words.append(document.createTextNode(wordText));
      if (entry.bingo) words.append(el("span", { class: "bingo" }, "BINGO"));
      li.append(words);
      li.append(el("div", { class: "delta" }, `+${entry.score}`));
    } else {
      const cls = entry.error ? "event error" : "event";
      li.append(el("div", { class: cls }, entry.text));
    }
    log.append(li);
  }
}

// ------------------------------------------------------------ interactions

function onRackClick(idx: number) {
  if (Date.now() < suppressClickUntil) return;
  if (state.exchangeMode) {
    if (state.exchangeSelection.has(idx)) state.exchangeSelection.delete(idx);
    else state.exchangeSelection.add(idx);
    render();
    return;
  }
  if (state.pending.some(p => p.rackIdx === idx)) return;
  state.selectedRackIdx = state.selectedRackIdx === idx ? -1 : idx;
  render();
}

async function onBoardClick(row: number, col: number) {
  if (state.selectedRackIdx < 0) return;
  if (state.pending.some(p => p.row === row && p.col === col)) return;
  if (state.board[row]?.[col]) return;

  const raw = state.yourRack[state.selectedRackIdx];
  const parsed = parseTileWire(raw);
  let letter = parsed.letter;
  const blank = parsed.blank;

  if (blank) {
    const chosen = await pickBlankLetter();
    if (!chosen) return;
    letter = chosen;
  }

  state.pending.push({ rackIdx: state.selectedRackIdx, row, col, letter, blank });
  state.selectedRackIdx = -1;
  render();
}

function onPendingClick(row: number, col: number) {
  if (Date.now() < suppressClickUntil) return;
  state.pending = state.pending.filter(p => !(p.row === row && p.col === col));
  render();
}

// ------------------------------------------------------------ drag and drop

function startDrag(e: PointerEvent, source: DragSource, letter: string, blank: boolean) {
  if (state.exchangeMode) return;
  if (e.button !== undefined && e.button !== 0) return;
  e.preventDefault();
  const sourceEl = e.currentTarget as HTMLElement | SVGElement;
  try { sourceEl.setPointerCapture(e.pointerId); } catch { /* some browsers */ }
  drag = {
    source,
    sourceEl,
    letter,
    blank,
    startX: e.clientX,
    startY: e.clientY,
    pointerId: e.pointerId,
    ghost: null,
    active: false,
    hintEl: null,
    rackHint: false,
  };
}

function activateDrag() {
  if (!drag) return;
  drag.active = true;

  const ghost = document.createElement("div");
  ghost.className = "tile-ghost" + (drag.blank ? " blank" : "");
  ghost.textContent = drag.letter || "?";
  if (drag.letter && drag.letter !== "?" && !drag.blank) {
    const pts = document.createElement("span");
    pts.className = "pts";
    pts.textContent = String(tilePoints(drag.letter, false));
    ghost.appendChild(pts);
  }
  document.body.appendChild(ghost);
  drag.ghost = ghost;
  drag.sourceEl.classList.add("dragging-source");
}

function onDocPointerMove(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return;
  if (!drag.active) {
    const dx = e.clientX - drag.startX;
    const dy = e.clientY - drag.startY;
    if (Math.hypot(dx, dy) > DRAG_THRESHOLD) {
      activateDrag();
    } else {
      return;
    }
  }
  if (!drag.ghost) return;
  drag.ghost.style.left = `${e.clientX}px`;
  drag.ghost.style.top = `${e.clientY}px`;
  updateDropHint(e.clientX, e.clientY);
}

function onDocPointerUp(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return;
  if (drag.active) {
    performDrop(e.clientX, e.clientY);
    suppressClickUntil = Date.now() + 250;
  }
  endDrag();
}

function onDocPointerCancel(e: PointerEvent) {
  if (!drag || e.pointerId !== drag.pointerId) return;
  endDrag();
}

function endDrag() {
  if (!drag) return;
  drag.sourceEl.classList.remove("dragging-source");
  clearDropHint();
  drag.ghost?.remove();
  drag = null;
}

function updateDropHint(x: number, y: number) {
  if (!drag) return;
  clearDropHint();

  const target = findBoardSquareAt(x, y);
  if (target && canDropAt(target)) {
    const rect = boardRectAt(target.row, target.col);
    if (rect) {
      rect.classList.add("drop-hint");
      drag.hintEl = rect;
    }
    return;
  }

  if (drag.source.kind === "board" && overRack(x, y)) {
    const rack = document.querySelector(".rack");
    rack?.classList.add("drop-hint");
    drag.rackHint = true;
  }
}

function clearDropHint() {
  if (!drag) return;
  drag.hintEl?.classList.remove("drop-hint");
  drag.hintEl = null;
  if (drag.rackHint) {
    document.querySelector(".rack")?.classList.remove("drop-hint");
    drag.rackHint = false;
  }
}

function canDropAt(target: { row: number; col: number }): boolean {
  if (!drag) return false;
  if (state.board[target.row]?.[target.col]) return false;
  const sameAsSource =
    drag.source.kind === "board" && drag.source.row === target.row && drag.source.col === target.col;
  if (sameAsSource) return true;
  return !state.pending.some(p => p.row === target.row && p.col === target.col);
}

function findBoardSquareAt(x: number, y: number): { row: number; col: number } | null {
  let node = document.elementFromPoint(x, y) as Element | null;
  while (node && !(node instanceof Element && node.hasAttribute("data-row"))) {
    node = node.parentElement;
  }
  if (!node) return null;
  const row = node.getAttribute("data-row");
  const col = node.getAttribute("data-col");
  if (row === null || col === null) return null;
  return { row: +row, col: +col };
}

function boardRectAt(row: number, col: number): SVGRectElement | null {
  return document.querySelector(`rect[data-row="${row}"][data-col="${col}"]`);
}

function overRack(x: number, y: number): boolean {
  const rack = document.querySelector(".rack");
  if (!rack) return false;
  const r = rack.getBoundingClientRect();
  return x >= r.left && x <= r.right && y >= r.top && y <= r.bottom;
}

async function performDrop(x: number, y: number) {
  if (!drag) return;
  const dragSnapshot = drag;
  const target = findBoardSquareAt(x, y);

  if (target && canDropAt(target)) {
    if (dragSnapshot.source.kind === "rack") {
      let letter = dragSnapshot.letter;
      if (dragSnapshot.blank) {
        const chosen = await pickBlankLetter();
        if (!chosen) {
          render();
          return;
        }
        letter = chosen;
      }
      state.pending.push({
        rackIdx: dragSnapshot.source.idx,
        row: target.row,
        col: target.col,
        letter,
        blank: dragSnapshot.blank,
      });
    } else {
      const fromRow = dragSnapshot.source.row;
      const fromCol = dragSnapshot.source.col;
      const moving = state.pending.find(p => p.row === fromRow && p.col === fromCol);
      if (moving) {
        moving.row = target.row;
        moving.col = target.col;
      }
    }
    state.selectedRackIdx = -1;
    render();
    return;
  }

  if (dragSnapshot.source.kind === "board" && overRack(x, y)) {
    const fromRow = dragSnapshot.source.row;
    const fromCol = dragSnapshot.source.col;
    state.pending = state.pending.filter(p => !(p.row === fromRow && p.col === fromCol));
    render();
  }
}

function submitMove() {
  const placements: PlacementWire[] = state.pending.map(p => ({
    row: p.row, col: p.col, letter: p.letter, blank: p.blank,
  }));
  send({ type: "place", placements });
  state.pending = [];
}

function confirmExchange() {
  const indices = [...state.exchangeSelection].sort((a, b) => a - b);
  send({ type: "exchange", indices });
  state.exchangeMode = false;
  state.exchangeSelection.clear();
}

// ------------------------------------------------------------ modals

function modalRoot(): HTMLElement { return $("modalRoot"); }

function showModal(card: HTMLElement): HTMLElement {
  const backdrop = el("div", { class: "modal-backdrop" });
  backdrop.append(card);
  modalRoot().append(backdrop);
  return backdrop;
}

function closeModal(node: HTMLElement) {
  node.remove();
}

function pickBlankLetter(): Promise<string | null> {
  return new Promise(resolve => {
    const card = el("div", { class: "modal-card" });
    card.append(el("h2", {}, "Choose a letter"));
    card.append(el("p", {}, "This blank tile will play as the letter you pick (scoring 0 points)."));

    const grid = el("div", { class: "blank-grid" });
    let resolved = false;
    const done = (letter: string | null) => {
      if (resolved) return;
      resolved = true;
      closeModal(backdrop);
      resolve(letter);
    };

    for (let code = "A".charCodeAt(0); code <= "Z".charCodeAt(0); code++) {
      const ch = String.fromCharCode(code);
      const btn = el("button", { type: "button" }, ch);
      btn.addEventListener("click", () => done(ch));
      grid.append(btn);
    }
    card.append(grid);

    const actions = el("div", { class: "actions" });
    const cancel = el("button", { type: "button" }, "Cancel");
    cancel.addEventListener("click", () => done(null));
    actions.append(cancel);
    card.append(actions);

    const backdrop = showModal(card);
  });
}

function showJoinModal(defaults: { name: string; room: string; lang: string }): Promise<{ name: string; room: string; lang: string }> {
  return new Promise(resolve => {
    const card = el("div", { class: "modal-card" });
    card.append(el("h2", {}, "Join a game"));
    card.append(el("p", {}, "Pick a display name. Enter a room code to join friends, or leave blank to create a new room."));

    const form = el("form", {});

    form.append(el("label", { htmlFor: "join-name" }, "Your name"));
    const nameInput = el("input", { type: "text", id: "join-name", placeholder: "Player", autocomplete: "off" }) as HTMLInputElement;
    nameInput.value = defaults.name;
    form.append(nameInput);

    const row = el("div", { class: "row" });
    const roomCol = el("div", {});
    roomCol.append(el("label", { htmlFor: "join-room" }, "Room code (optional)"));
    const roomInput = el("input", { type: "text", id: "join-room", placeholder: "e.g. ABCD", autocomplete: "off", maxLength: "6" }) as HTMLInputElement;
    roomInput.value = defaults.room;
    roomInput.style.textTransform = "uppercase";
    roomCol.append(roomInput);
    row.append(roomCol);

    const langCol = el("div", {});
    langCol.append(el("label", { htmlFor: "join-lang" }, "Language"));
    const langSelect = el("select", { id: "join-lang" }) as HTMLSelectElement;
    langSelect.append(el("option", { value: "en" }, "English"));
    langSelect.value = defaults.lang;
    langCol.append(langSelect);
    row.append(langCol);
    form.append(row);

    const actions = el("div", { class: "actions" });
    const submit = el("button", { type: "submit", class: "primary" }, "Join");
    actions.append(submit);
    form.append(actions);

    let resolved = false;
    form.addEventListener("submit", (e) => {
      e.preventDefault();
      if (resolved) return;
      const name = nameInput.value.trim() || "Player";
      const room = roomInput.value.trim().toUpperCase();
      const lang = langSelect.value;
      resolved = true;
      closeModal(backdrop);
      resolve({ name, room, lang });
    });

    card.append(form);
    const backdrop = showModal(card);
    setTimeout(() => nameInput.focus(), 50);
  });
}

function showGameOverOverlay() {
  const sorted = [...state.players].sort((a, b) => b.score - a.score);
  const winnerID = sorted[0]?.id;

  const card = el("div", { class: "modal-card" });
  card.append(el("h2", {}, "Game Over"));
  if (sorted[0]) {
    card.append(el("p", {}, `${sorted[0].name} wins with ${sorted[0].score} points.`));
  }

  const tbl = el("table", { class: "gameover-table" });
  for (const p of sorted) {
    const row = el("tr");
    row.append(el("td", { class: p.id === winnerID ? "winner" : "" }, p.name + (p.bot ? " (bot)" : "")));
    row.append(el("td", { class: (p.id === winnerID ? "winner " : "") + "num" }, String(p.score)));
    tbl.append(row);
  }
  card.append(tbl);

  if (state.leaderboard.length > 0) {
    card.append(el("h3", { style: "margin: 20px 0 8px; font-size: 12px; color: var(--muted); letter-spacing: 1px; text-transform: uppercase;" }, "All-time leaderboard"));
    const lbtbl = el("table", { class: "gameover-table" });
    for (const entry of state.leaderboard.slice(0, 5)) {
      const row = el("tr");
      row.append(el("td", {}, entry.name || entry.playerID));
      row.append(el("td", { class: "num" }, `${entry.wins} win${entry.wins === 1 ? "" : "s"}`));
      lbtbl.append(row);
    }
    card.append(lbtbl);
  }

  const actions = el("div", { class: "actions" });
  const playAgain = el("button", { class: "primary" }, "Play Again");
  playAgain.addEventListener("click", () => { closeModal(backdrop); state.gameOverShown = false; send({ type: "playAgain" }); });
  const close = el("button", {}, "Close");
  close.addEventListener("click", () => closeModal(backdrop));
  actions.append(playAgain, close);
  card.append(actions);

  const backdrop = showModal(card);
}

// ------------------------------------------------------------ ws + msgs

function send(msg: Record<string, unknown>) {
  if (!state.ws || state.ws.readyState !== WebSocket.OPEN) return;
  state.ws.send(JSON.stringify(msg));
}

function handle(msg: Msg) {
  switch (msg.type) {
    case "joined":
      state.playerID = msg.playerID;
      state.roomCode = msg.room;
      state.language = msg.language;
      state.owner = msg.owner;
      if (msg.leaderboard) state.leaderboard = msg.leaderboard;
      render();
      break;
    case "state":
      state.phase = msg.phase;
      state.board = msg.board;
      state.yourRack = msg.yourRack || [];
      state.players = msg.players;
      state.currentID = msg.currentID;
      state.ownerID = msg.ownerID;
      state.bagRemaining = msg.bagRemaining;
      state.turnDeadlineMs = msg.timerMs > 0 ? Date.now() + msg.timerMs : 0;
      state.owner = state.playerID === msg.ownerID;
      if (msg.phase !== "gameOver") state.gameOverShown = false;
      state.pending = state.pending.filter(p => !state.board[p.row]?.[p.col]);
      render();
      break;
    case "move": {
      state.log.push({
        kind: "move",
        playerID: msg.playerID,
        name: msg.name,
        words: msg.words,
        score: msg.score,
        bingo: msg.bingo,
      });
      if (msg.placements && msg.placements.length > 0) {
        state.lastMove = { placements: msg.placements, expiresAt: Date.now() + LAST_MOVE_HIGHLIGHT_MS };
        setTimeout(() => {
          if (state.lastMove && Date.now() >= state.lastMove.expiresAt) {
            state.lastMove = null;
            render();
          }
        }, LAST_MOVE_HIGHLIGHT_MS + 50);
      }
      render();
      break;
    }
    case "chat":
      state.log.push({ kind: "event", text: `${msg.from} ${msg.text}` });
      render();
      break;
    case "error":
      state.log.push({ kind: "event", text: msg.message, error: true });
      render();
      break;
    case "gameOver":
      if (msg.leaderboard) state.leaderboard = msg.leaderboard;
      break;
  }
}

// ------------------------------------------------------------ timer tick

function tickTimer() {
  const t = $("timer");
  if (state.phase === "paused") {
    t.textContent = "⏸";
    t.className = "timer warn";
    return;
  }
  if (state.turnDeadlineMs === 0 || state.phase !== "playing") {
    t.textContent = "";
    t.className = "timer";
    return;
  }
  const remaining = Math.max(0, state.turnDeadlineMs - Date.now());
  const seconds = Math.ceil(remaining / 1000);
  t.textContent = `${seconds}s`;
  t.className = "timer" + (seconds <= 10 ? " crit" : seconds <= 30 ? " warn" : "");
}

// ------------------------------------------------------------ init

function getOrCreatePlayerID(): string {
  let id = sessionStorage.getItem("scrabble.playerID");
  if (!id) {
    id = crypto.randomUUID();
    sessionStorage.setItem("scrabble.playerID", id);
  }
  return id;
}

function connectWS(name: string, room: string, lang: string) {
  state.name = name;
  state.language = lang;
  state.playerID = getOrCreatePlayerID();
  sessionStorage.setItem("scrabble.name", name);

  $("app").hidden = false;

  const url = new URL(window.location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = "/ws";
  url.search = new URLSearchParams({ name, id: state.playerID, room, lang }).toString();

  const ws = new WebSocket(url.toString());
  state.ws = ws;
  ws.addEventListener("message", e => {
    try { handle(JSON.parse(e.data)); } catch { /* ignore */ }
  });
  ws.addEventListener("close", () => {
    state.log.push({ kind: "event", text: "disconnected", error: true });
    render();
  });

  document.addEventListener("pointermove", onDocPointerMove);
  document.addEventListener("pointerup", onDocPointerUp);
  document.addEventListener("pointercancel", onDocPointerCancel);

  setInterval(tickTimer, 250);
  render();
}

async function init() {
  const params = new URLSearchParams(window.location.search);
  const urlName = params.get("name") ?? "";
  const urlRoom = params.get("room") ?? "";
  const urlLang = params.get("lang") ?? "en";

  if (urlName) {
    connectWS(urlName, urlRoom, urlLang);
    return;
  }

  const cached = sessionStorage.getItem("scrabble.name") ?? "";
  const choice = await showJoinModal({ name: cached, room: urlRoom, lang: urlLang });
  connectWS(choice.name, choice.room, choice.lang);
}

init();
