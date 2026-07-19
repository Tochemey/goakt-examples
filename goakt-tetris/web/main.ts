// Tetris client. Renders the server-authoritative state via a 2D canvas
// and sends every keypress as a discrete action over the WebSocket.

// ─── Wire-protocol types ────────────────────────────────────────────────
//
// These mirror the JSON shapes emitted by the Go server (see types.go).
// JSON.parse returns `any`, so the deserialized snapshot is asserted as
// Snapshot at the WS message boundary — no runtime validation here. That's
// fine for a same-team-controlled protocol; a stricter client would gate
// the assertion behind a schema check.

interface Piece {
  kind: number; // 0..6 → I, O, T, S, Z, J, L
  rot: number;  // 0..3
  x: number;
  y: number;
}

interface Snapshot {
  tick: number;
  t: number;
  grid: number[][];
  piece: Piece;
  ghostY: number;
  next: number;
  score: number;
  lines: number;
  level: number;
  gameOver: boolean;
  paused: boolean;
}

const MSG_TYPE_INPUT = "input";

// Action strings — mirror of the Go constants in types.go. Changing either
// side without the other will break the protocol.
const ACTION = {
  LEFT:          "left",
  RIGHT:         "right",
  ROTATE:        "rotate",
  SOFT_DROP:     "softdrop",
  SOFT_DROP_END: "softdrop-end",
  HARD_DROP:     "harddrop",
  PAUSE:         "pause",
  RESTART:       "restart",
} as const;

type Action = typeof ACTION[keyof typeof ACTION];

// ─── Render constants ───────────────────────────────────────────────────

// Piece kind → hex color. Index 0 = empty, 1..7 = I, O, T, S, Z, J, L
// (matches the order in match.go's pieceShapes table).
const COLORS: readonly string[] = [
  "#000",     // 0 = empty (unused — empty cells aren't drawn)
  "#22d3ee",  // 1 = I (cyan)
  "#facc15",  // 2 = O (yellow)
  "#c026d3",  // 3 = T (magenta)
  "#22c55e",  // 4 = S (green)
  "#ef4444",  // 5 = Z (red)
  "#3b82f6",  // 6 = J (blue)
  "#fb923c",  // 7 = L (orange)
];

// Mirror of pieceShapes in match.go. Each piece's base orientation as 4
// (dx, dy) cell offsets from its pivot.
type Cell = readonly [number, number];
const SHAPES: readonly Cell[][] = [
  [[-1, 0], [0, 0], [1, 0], [2, 0]],   // I
  [[0, 0],  [1, 0], [0, 1], [1, 1]],   // O
  [[-1, 0], [0, 0], [1, 0], [0, 1]],   // T
  [[0, 0],  [1, 0], [-1, 1], [0, 1]],  // S
  [[-1, 0], [0, 0], [0, 1], [1, 1]],   // Z
  [[-1, 0], [0, 0], [1, 0], [1, 1]],   // J
  [[-1, 0], [0, 0], [1, 0], [-1, 1]],  // L
];

function rotateCells(cells: readonly Cell[], rot: number): Cell[] {
  let out: Cell[] = cells.map(c => [c[0], c[1]]);
  for (let i = 0; i < rot; i++) {
    out = out.map(([x, y]): Cell => [-y, x]);
  }
  return out;
}

const CELL = 30;
const BOARD_W = 10;
const BOARD_H = 20;

// ─── DOM lookups ────────────────────────────────────────────────────────

function el<T extends HTMLElement>(id: string): T {
  const found = document.getElementById(id);
  if (!found) throw new Error(`missing #${id} in DOM`);
  return found as T;
}
function canvasCtx(c: HTMLCanvasElement): CanvasRenderingContext2D {
  const ctx = c.getContext("2d");
  if (!ctx) throw new Error("2D canvas context unavailable");
  return ctx;
}

const board = el<HTMLCanvasElement>("board");
const bctx = canvasCtx(board);
const next = el<HTMLCanvasElement>("nextCanvas");
const nctx = canvasCtx(next);
const scorePanel = el("scorePanel");
const linesPanel = el("linesPanel");
const levelPanel = el("levelPanel");
const scoreVal = el("scoreVal");
const linesVal = el("linesVal");
const levelVal = el("levelVal");
const overlay = el("overlay");
const pauseOverlay = el("pauseOverlay");
const pauseBtn = el<HTMLButtonElement>("pauseBtn");
const statusEl = el("status");

pauseBtn.addEventListener("click", () => {
  send(ACTION.PAUSE);
  pauseBtn.blur(); // so Space doesn't re-trigger the button
});

// ─── State + WebSocket ──────────────────────────────────────────────────

let state: Snapshot | null = null;
let ws: WebSocket | null = null;

function connect(): void {
  const proto = location.protocol === "https:" ? "wss:" : "ws:";
  ws = new WebSocket(`${proto}//${location.host}/ws`);
  ws.onopen = () => { statusEl.textContent = "connected"; };
  ws.onclose = () => {
    ws = null;
    statusEl.textContent = "disconnected — retrying";
    setTimeout(connect, 1000);
  };
  ws.onerror = () => ws?.close();
  ws.onmessage = (e: MessageEvent<string>) => {
    state = JSON.parse(e.data) as Snapshot;
    render();
  };
}
connect();

function send(action: Action): void {
  if (ws && ws.readyState === WebSocket.OPEN) {
    ws.send(JSON.stringify({ type: MSG_TYPE_INPUT, action }));
  }
}

// Browser auto-repeats keydown while a key is held, which gives natural
// auto-repeat for left/right/soft-drop. Rotate / harddrop / restart fire
// once per physical press.
addEventListener("keydown", (e: KeyboardEvent) => {
  const repeatable = e.code === "ArrowLeft"  || e.code === "KeyA"
                  || e.code === "ArrowRight" || e.code === "KeyD"
                  || e.code === "ArrowDown"  || e.code === "KeyS";
  if (e.repeat && !repeatable) return;

  switch (e.code) {
    case "ArrowLeft":  case "KeyA": send(ACTION.LEFT); break;
    case "ArrowRight": case "KeyD": send(ACTION.RIGHT); break;
    case "ArrowUp":    case "KeyW": send(ACTION.ROTATE); break;
    case "ArrowDown":  case "KeyS": send(ACTION.SOFT_DROP); break;
    case "Space":                   send(ACTION.HARD_DROP); break;
    case "KeyP":                    send(ACTION.PAUSE); break;
    case "KeyR":                    send(ACTION.RESTART); break;
    default: return;
  }
  e.preventDefault();
});

addEventListener("keyup", (e: KeyboardEvent) => {
  if (e.code === "ArrowDown" || e.code === "KeyS") {
    send(ACTION.SOFT_DROP_END);
  }
});

// ─── Render ─────────────────────────────────────────────────────────────

function render(): void {
  if (!state) return;
  const s = state;

  bctx.clearRect(0, 0, board.width, board.height);
  drawGridLines(bctx);

  for (let y = 0; y < s.grid.length; y++) {
    const row = s.grid[y]!;
    for (let x = 0; x < row.length; x++) {
      const v = row[x]!;
      if (v !== 0) drawCell(bctx, x, y, v);
    }
  }

  if (!s.gameOver) {
    const cells = rotateCells(SHAPES[s.piece.kind]!, s.piece.rot);
    const dropDist = s.ghostY - s.piece.y;
    if (dropDist > 0) {
      for (const [dx, dy] of cells) {
        const gx = s.piece.x + dx;
        const gy = s.ghostY + dy;
        if (gy >= 0 && gy < s.grid.length) {
          drawGhostCell(bctx, gx, gy, s.piece.kind + 1);
        }
      }
    }
    for (const [dx, dy] of cells) {
      const x = s.piece.x + dx;
      const y = s.piece.y + dy;
      if (y >= 0) drawCell(bctx, x, y, s.piece.kind + 1);
    }
  }

  // Next-piece preview, centered on its bounding box.
  nctx.clearRect(0, 0, next.width, next.height);
  const np = SHAPES[s.next]!;
  let minX = Infinity, maxX = -Infinity, minY = Infinity, maxY = -Infinity;
  for (const [dx, dy] of np) {
    if (dx < minX) minX = dx;
    if (dx > maxX) maxX = dx;
    if (dy < minY) minY = dy;
    if (dy > maxY) maxY = dy;
  }
  const previewCell = 20;
  const offsetX = next.width / 2 - ((minX + maxX) / 2) * previewCell;
  const offsetY = next.height / 2 - ((minY + maxY) / 2) * previewCell;
  for (const [dx, dy] of np) {
    drawSmallCell(nctx,
      offsetX + dx * previewCell - previewCell / 2,
      offsetY + dy * previewCell - previewCell / 2,
      previewCell, s.next + 1);
  }

  scoreVal.textContent = s.score.toLocaleString();
  linesVal.textContent = String(s.lines);
  levelVal.textContent = String(s.level);
  pulseOnChange(scorePanel, s.score, "lastScore");
  pulseOnChange(linesPanel, s.lines, "lastLines");
  pulseOnChange(levelPanel, s.level, "lastLevel");

  overlay.classList.toggle("show", s.gameOver);
  pauseOverlay.classList.toggle("show", s.paused && !s.gameOver);
  pauseBtn.textContent = s.paused ? "Resume" : "Pause";
}

function pulseOnChange(elt: HTMLElement, current: number, key: string): void {
  const prev = elt.dataset[key];
  if (prev !== undefined && Number(prev) !== current) {
    elt.classList.remove("pulse");
    void elt.offsetWidth; // force reflow so the animation restarts
    elt.classList.add("pulse");
  }
  elt.dataset[key] = String(current);
}

function drawGridLines(ctx: CanvasRenderingContext2D): void {
  ctx.strokeStyle = "rgba(255, 255, 255, 0.035)";
  ctx.lineWidth = 1;
  for (let x = 1; x < BOARD_W; x++) {
    ctx.beginPath();
    ctx.moveTo(x * CELL + 0.5, 0);
    ctx.lineTo(x * CELL + 0.5, BOARD_H * CELL);
    ctx.stroke();
  }
  for (let y = 1; y < BOARD_H; y++) {
    ctx.beginPath();
    ctx.moveTo(0, y * CELL + 0.5);
    ctx.lineTo(BOARD_W * CELL, y * CELL + 0.5);
    ctx.stroke();
  }
}

function drawCell(ctx: CanvasRenderingContext2D, x: number, y: number, val: number): void {
  const px = x * CELL;
  const py = y * CELL;
  const c = COLORS[val]!;

  const grad = ctx.createLinearGradient(px, py, px + CELL, py + CELL);
  grad.addColorStop(0, shade(c, 35));
  grad.addColorStop(1, shade(c, -25));
  ctx.fillStyle = grad;
  ctx.fillRect(px + 1, py + 1, CELL - 2, CELL - 2);

  ctx.fillStyle = "rgba(255, 255, 255, 0.35)";
  ctx.fillRect(px + 1, py + 1, CELL - 2, 2);
  ctx.fillRect(px + 1, py + 1, 2, CELL - 2);

  ctx.fillStyle = "rgba(0, 0, 0, 0.4)";
  ctx.fillRect(px + 1, py + CELL - 3, CELL - 2, 2);
  ctx.fillRect(px + CELL - 3, py + 1, 2, CELL - 2);
}

function drawGhostCell(ctx: CanvasRenderingContext2D, x: number, y: number, val: number): void {
  const px = x * CELL;
  const py = y * CELL;
  ctx.strokeStyle = COLORS[val]! + "70"; // ~44% alpha
  ctx.lineWidth = 2;
  ctx.strokeRect(px + 3, py + 3, CELL - 6, CELL - 6);
}

function drawSmallCell(ctx: CanvasRenderingContext2D, px: number, py: number, size: number, val: number): void {
  const c = COLORS[val]!;
  const grad = ctx.createLinearGradient(px, py, px + size, py + size);
  grad.addColorStop(0, shade(c, 35));
  grad.addColorStop(1, shade(c, -25));
  ctx.fillStyle = grad;
  ctx.fillRect(px + 1, py + 1, size - 2, size - 2);
  ctx.fillStyle = "rgba(255, 255, 255, 0.3)";
  ctx.fillRect(px + 1, py + 1, size - 2, 1);
  ctx.fillRect(px + 1, py + 1, 1, size - 2);
}

// shade brightens (positive amt) or darkens (negative) a #rrggbb hex color.
function shade(hex: string, amt: number): string {
  const n = parseInt(hex.slice(1), 16);
  const clamp = (v: number) => Math.max(0, Math.min(255, v));
  const r = clamp(((n >> 16) & 0xff) + amt);
  const g = clamp(((n >>  8) & 0xff) + amt);
  const b = clamp((n & 0xff) + amt);
  return `rgb(${r}, ${g}, ${b})`;
}
