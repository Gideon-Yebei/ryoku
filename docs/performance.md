# Performance

What the Ryoku desktop costs while it runs, how that was measured, and what the
October 2026 pass changed. The numbers are from one machine; the method is the
part to reuse.

## How it was measured

**The machine.** Ryzen 7 7940HS with an RTX 4060 in MUX discrete mode (the
panel is driven by the NVIDIA GPU), one 2560×1600 panel at 165 Hz and scale
1.333, niri, NVIDIA 615.78.08, the Performance power profile for every run. The
user's real desktop was measured, not a fresh install: Orpheus with the Python
dock, the line visualizer at the top edge, iRiS and ii desktop widgets, a still
wallpaper and a paired phone.

**The noise.** Everything unrelated was stopped first (Vesktop, a Discord bot,
both Ollama services, a local LLM container). The terminal multiplexer and the
coding agent that drove the runs are excluded from every sum.

**The tool.** `bin/ryoku-dev-perf-baseline` samples every process the desktop
owns for one window (60 s here) and records, per process and per group:

- proportional memory (PSS, never summed RSS: shared Qt and driver pages would
  count once per process);
- CPU per thread and wakeups (context switches per second);
- GPU time and VRAM from `nvidia-smi pmon` and the DRM fdinfo counters;
- package and core power (RAPL), the amdgpu PPT sensor and NVIDIA board power;
- the active power profile, because Power Saver changes clocks and the shell's
  own effects;
- with `--spawns`, every process launch per minute and who started it, from the
  kernel's exec tracepoint;
- with `--frames`, frames per shell window from Qt's render loop;
- with `--surfaces`, commits and presented frames per layer surface by
  namespace, from `WAYLAND_DEBUG`;
- with `--startup`, the time from a shell restart to its configuration loading.

It asks the compositor nothing, so it reads the same under every compositor.
The debug modes restart the shell with a log that costs CPU itself: take their
frame counts, not their CPU.

**The scenarios.** Each ran after a shell restart and a 90 to 120 s settle, on an
empty workspace:

- *idle*: nothing playing;
- *music*: mpv playing into a muted sink, so the analysers, the Orpheus drops
  and the visualizer all work as they do with sound;
- *visualizer off*: idle with the line visualizer disabled;
- *sweep*: every built-in bar style at idle;
- *startup*: three shell restarts.

**The base desktop** is everything Ryoku ships and runs for everyone: the shell
UI (`qs`), the shell daemon, its helpers (cava, pw-record), the window-manager
provider, the wallpaper daemon, the compositor, session daemons and Ryoku's
system units. Optional features are reported apart: phone integration,
dictation, Rashin and Prowl, Cobalt.

## Results

Measured on 2026-10-22. CPU is a percentage of one core, averaged over the 60 s
window. "Base" is the whole base desktop; `qs` is the shell process alone, the
part that draws. Before is unstable-dev at `da60f0d30`. The idle rows after come
from the first full run of this branch, the music rows from the second, which
also carries the C++ spectrum and the shared Orpheus clock; the second run's
idle sweep is left out because the terminal driving it was busy, which shows up
in niri and in the power readings.

| scenario | base CPU | `qs` CPU | `qs` wakeups/s | niri CPU | launches/min | package W | NVIDIA W |
|---|---:|---:|---:|---:|---:|---:|---:|
| Orpheus, idle | 6.17 → 4.12 | 2.85 → 3.13 | 123 → 83 | 1.13 → 0.63 | 5,796 → 13 | 15.8 → 12.7 | 4.36 → 4.98 |
| Orpheus, music | 81.0 → 60.4 | 63.6 → 47.6 | 5,978 → 3,778 | 11.7 → 10.4 | 5,954 → 16 | 26.4 → 23.9 | 16.0 → 10.6 |
| Orpheus, visualizer off | 6.22 → 2.77 | 2.97 → 1.80 | 118 → 33 | 1.12 → 0.43 | 5,311 → 13 | 15.3 → 12.4 | 4.42 → 4.85 |
| Kairos, idle | 4.57 → 2.25 | 2.22 → 1.53 | 72 → 27 | 0.25 → 0.23 | 5,411 → 13 | 15.0 → 12.5 | 3.65 → 4.89 |
| Kairos, music | 60.0 → 38.2 | 44.9 → 28.5 | 4,388 → 1,897 | 10.2 → 7.3 | 5,446 → 16 | 24.6 → 19.1 | 11.4 → 10.8 |

**Frames.** On a 165 Hz panel the shell's windows presented 5.3 frames a second
at idle before and 1.8 after. With music, 404 before (two full-screen windows
at the panel's rate) and 273 after; per surface, the Orpheus bar (now 84 px
tall) presents about 150, the desktop with its iRiS visualizer about 65 and the
line visualizer about 55.

**Startup.** From a shell restart to its configuration loaded: 3.93 s before,
3.27 to 3.31 s after.

**Memory.** Measured back to back on the same base (`8a17af10c`), the shell
process holds 597 to 620 MB PSS on that base and 593 to 597 MB on this branch.
Most of the difference is the wallpaper's video stack, which a still wallpaper
no longer loads: 655 MB with it, 593 MB without. The wallpaper picker, kept
resident for an instant Super+W, holds another 116 to 131 MB.

**The bar styles at idle**, same conditions, before → after:

| style | base CPU | `qs` CPU | `qs` wakeups/s |
|---|---:|---:|---:|
| Orpheus | 6.17 → 4.12 | 2.85 → 3.13 | 123 → 83 |
| Kairos | 4.57 → 2.25 | 2.22 → 1.53 | 72 → 27 |
| QS Bar | 15.7 → 12.7 | 11.3 → 10.7 | 426 → 245 |
| Python | 17.3 → 2.42 | 12.9 → 1.62 | 711 → 28 |
| Nomarchy | 4.84 → 2.24 | 2.56 → 1.49 | 58 → 24 |
| Shima | 4.62 → 3.04 | 2.22 → 2.09 | 59 → 35 |
| Sumi | 4.49 → 2.24 | 2.11 → 1.60 | 56 → 24 |

Two readings did not improve. Orpheus's own idle CPU is within noise of before
(the bar now waits on fewer timers, but its windows' render threads still wake),
and the NVIDIA board drew about half a watt more at idle in the after runs
while no Ryoku process used measurably more GPU time; the GPU ran 5 °C warmer,
and this data does not separate the two.

## Where the cost was

Most of it was not drawing. It was processes started on timers, analysers and
captures nobody looked at, and windows redrawn whole for a small change.

**Process launches.** An idle desktop started about 5,800 processes a minute.
Almost all of it was the login-screen guard, which walked `/proc` every five
seconds and forked `tr` for every entry. The lid daemon ran `awk` twice a second,
the shell daemon ran `pgrep`, `ryoku-host` and `systemctl` every five seconds,
and the shell itself ran `nvidia-smi`, `sh`, `adb`, `awk` and `gsr-cli` on
timers. Each of these now reads the kernel directly or waits for an event: the
guard and the lid daemon read `/proc` with shell builtins, the daemon scans
`/proc` in-process and asks the user manager over D-Bus, system stats come from
one native sampler, and the phone module follows KDE Connect and network events
instead of polling `adb`.

**Audio.** Music ran three cava processes, a Python waveform capture and
`pw-record`, each feeding its own consumer. One analyser in C++ (`CavaFeed`) now
feeds the Orpheus drops, the voice bars, the desktop widgets and the iNiR
services, and the line visualizer reads a C++ capture (`WaveCapture`) instead of
Python. Both stop in silence, and the visualizer rests instead of ticking.

**Whole-window redraws.** Orpheus kept a full-screen window at all times, so
every frame of its drops re-rendered and recomposited the whole output, at the
panel's refresh rate while music played. It now rests at bar height and grows
only for a card, its settings or a morph, and its drops request at most about 60
frames a second. The spinning cover and the seek squiggle step on one 30 Hz
clock instead of running animations at 165 Hz. The desktop re-rendered the full
screen every second for its clock; minute faces now read a minute clock. The
desktop visualizer widget was a JavaScript Canvas that cost about three
full-window frames per analyser update; as a C++ painter it costs two (see the
layer note below), and none of the drawing runs in JavaScript.

**Idle redraws in the faces.** Python faces restarted an 800 ms ease for
sub-percent sensor jitter, the battery wave ran forever, material clocks woke
every second without a seconds hand, and QS Bar's silent drift ran at 20 fps.
Each now changes only when what it shows changes.

**Docks.** Docks recomputed on every window title change and kept drawing while
hidden; they ignore title-only updates now, the Python dock unmaps behind an
edge strip, and the ii dock gates its magnification, previews and widgets on
being shown.

## What moved to C++

The QML stays presentation; the hot loops moved into the `Ryoku.Blobs` plugin
(`ryoku/shell/plugin/`), each with a single owner and consumer counting:

- `CavaFeed` reads cava's binary frames, with the peak follower and stale
  flattening the QML parser had;
- `WaveCapture` turns `pw-record`'s PCM into the scope's waveform;
- `SystemMonitor` serves CPU, memory, temperatures, GPU and network from the
  kernel on one worker, replacing QML `/proc` parsing and `nvidia-smi` calls;
- `CavaSpectrumPainter` is the iNiR spectrum's QPainter drawing as a painted
  item that repaints once per change;
- `BlobWave` caps its wave frames and stops once settled.

## Next to other Quickshell shells

A source study of Noctalia, Caelestia, DankMaterialShell, iNiR, end-4,
Serpantinum and Omarchy Desktop found the same lesson in each: the cost is
surface and service lifetime, not QML itself. The shells that idle well map
about two surfaces per output, start an analyser or sampler only for a visible
consumer, read the kernel natively and schedule frames on demand. Noctalia
(now a native C++ shell) runs PipeWire capture and FFT in process and goes
dormant after a second of silence; Caelestia embeds libcava in its plugin; DMS
runs six cava bars at 25 fps only while a visible widget plays. This pass moved
Ryoku onto those patterns where it was behind: one consumer-owned analyser,
native stats, no timers where an event exists, and windows sized to what they
draw.

Public resource reports for those shells (for example
[DMS #631](https://github.com/AvengeMedia/DankMaterialShell/issues/631), 7%
Quickshell plus 5% niri at idle, most of it cava, and
[Caelestia #1603](https://github.com/caelestia-dots/shell/issues/1603), about 4%
CPU for a continuously repainting background) are bug reports on other machines,
not baselines to rank against.

## What still costs

In the order to take them:

1. **Layered content presents twice.** Content that animates inside
   `layer.enabled` measured two presented frames per change: the layer's
   texture is refreshed in one frame and shown in the next. Orpheus applies its
   default 0.94 opacity to the composed body through a layer, so with music its
   bar presents about 150 frames a second, against about 88 with the layer off.
   The desktop's iRiS visualizer shows the same ratio, about 65 frames a second
   for a 30 Hz analyser. Folding the opacity
   into the blob shader, each pixel drawn by exactly one shape, would remove
   the layer without changing the look.
2. **The desktop visualizer redraws the whole desktop.** Its widget lives in
   the full-screen desktop surface, so each of its frames re-renders the
   output (about 65 a second with music). iNiR's own background lets such
   content move into a small surface of its own while nothing covers it
   (`LiveLayer`); the Ryoku desktop does not host that yet, and a scaled widget
   cannot use it.
3. **The line visualizer** draws at its configured 60 fps while music plays;
   30 halves it.
4. **A hidden overview backdrop presents about once a second**, behind the
   desktop, even with nothing in it changing.
5. **Memory per window.** The shell holds about 600 MB, and each mapped window
   has its own render thread, GL context and swapchain on top of the QML
   engine. Fewer, smaller surfaces is the lever, not QML size.
6. **The discrete GPU drives the panel** in MUX discrete mode, so the RTX 4060
   idles at 4 to 5 W whatever the shell does; Hybrid mode is the saving there.
7. **Startup** is 3.3 s to the configuration loaded; precompiling the QML
   (`docs/roadmap.md`, phase 1) is the next step.

Optional features carry their own cost and are off unless chosen: phone
integration (the KDE Connect monitor), dictation, Rashin and Prowl, Cobalt. QS
Bar stays the heaviest style: its ambient stream keeps it near 11% of a core
and a quarter of the GPU at idle on Performance.

## Measuring it yourself

```sh
ryoku-dev-perf-baseline --label idle --json idle.json            # 60 s, as the desktop is now
ryoku-dev-perf-baseline --restart --settle 120 --spawns          # who launches what
ryoku-dev-perf-baseline --surfaces --window 15 --settle 30       # which surface draws
ryoku-dev-perf-baseline --startup                                # restart to config loaded
ryoku-dev-perf-baseline --compare before.json after.json
```

Set the scenario up first (bar style, music, open surfaces), keep the power
profile the same between runs (`--compare` warns when it differs), and stop
anything heavy the desktop does not own.
