# case/ notes (moved from the root CLAUDE.md)

### 3D-printed case for the Sunton board (2026-10-16)

`case/sunton_2424s012_case.scad` (parametric, OpenSCAD 2021.01) and the exported
`sunton_2424s012_front.stl` / `_back.stl`; regenerate with `-D 'part="front"'` /
`'part="back"'` (`"both"` and a cutaway `"assembly"` exist for preview). Two parts: a
cup (bezel window, six glass guide pads, PCB pocket, internal thread, USB-C window,
FPC-tail slot; printed face-down) and a threaded plug back (flange + hollow 2-turn
right-hand skirt, coin slot; printed flat, thread up). 45.5 mm OD x 13.7 mm (`pcb_d` = 38.2). Print-tested
by the owner on the real board: fits, minimal rattle, a small gap remains at the glass.
Re-export both STLs after changing the `.scad`, and check each is a single valid solid
(cup and plug must not intersect). Things learned the hard way:

- The tail-slot cuts must be clipped to a cylinder inside the wall
  (`R_out - cup_wall`); unclipped rectangular cubes poked their corners through the
  round wall and looked like two vertical slits.
- A full bore ring caught the board at the FPC tail, so the glass is centred by six
  pads (`glass_clr` = 0.35 mm; going below ~0.2 risks gripping the glass).
- A thin band under the USB window snapped off prints; `usb_open_to_rim` makes it a
  notch open to the rim instead. The window is sized for a molded cable plug
  (`usb_plug_w/h` = 12 x 7 mm, plug ~11.5 x 6.5).
- The display still floats in depth; a foam pad behind the PCB is the owner's fix.
- `back_pedestal`: a full disc on the plug floor, `ped_gap` (0.3) below the tallest part
  (`back_h`), so the board can only sag back that far; the owner reports the current values
  (`pcb_d` 38.2, `ped_gap` 0.3) are ~99% right on the real hardware (2026-10-16). Posts were tried
  and dropped: the plug turns ~2 turns while rising, so any post at a radius sweeps the whole
  circle over the board's parts (module corners, buttons, switch and connectors reach the PCB
  edge) and would jam; the stop angle isn't fixed either. Only a disc below every part is safe.
- Unconfirmed: the real FPC tail angle relative to the USB-C receptacle (both default
  270 deg in `tail_angle`/`usb_angle`; photos suggest ~90-130 deg apart).

Desk stand: `case/sunton_2424s012_stand.scad` + `.stl` (print-tested by the owner, 2026-10-17,
"perfect"). Hull of a base slab and a cradle leaning back 15 deg; `case_d` (45.5) is hand-copied
from the case file, so update it if the case's outer diameter changes. Cable notches at
`exit_angles = [0, 180]`. The MiSTer-Kun mascot is embossed on the front slope from
`mister_kun_logo.scad`, a baked polygon of the official `mister_kun_bw.svg` black layer
(OpenSCAD's SVG import fills the whole silhouette because the file paints white over black);
credits in `mister_kun_CREDITS.txt`, not relicensed under GPL.
