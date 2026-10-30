# Case (Sunton ESP32-2424S012C)

![The printed case with a USB-C cable, showing the connected screen](case-photo.jpg)

`case/` has a small two-part screw-together case for the all-in-one board:
`sunton_2424s012_case.scad` (parametric OpenSCAD) and the exported
`sunton_2424s012_front.stl` / `sunton_2424s012_back.stl`.

- **Front (cup):** the bezel with a round window, six small pads that centre
  the glass, a pocket for the PCB, an internal thread, a USB-C opening and a
  slot for the display's flex tail. Print it **face-down**.
- **Back (plug):** a flat disc the same diameter as the cup, with a hollow
  threaded skirt that screws in from behind (right-hand thread, 2 turns) and a
  coin slot underneath. Print it **flat, thread up**.
- Outer diameter about 45.5 mm, height about 13.7 mm. The USB-C window is sized
  for a molded cable plug (about 11.5 x 6.5 mm) with a little play.
- Print-tested on a real board; it fits and the glass has only slight play. The
  display can still float a little in depth, so a small foam pad behind the PCB
  helps. The back has an optional flat pedestal (`back_pedestal`, `ped_gap`) just
  below the board's tallest parts, so the board can only sag back about 0.3 mm and
  the foam pad can be thin. The owner reports the current values (`pcb_d` 38.2,
  `ped_gap` 0.3) fit the real hardware almost perfectly; if the plug won't seat
  on yours, raise `ped_gap` or set `back_pedestal = false`.

Tolerances depend on your printer. The ones you're most likely to tune are at
the top of the `.scad` file: `glass_clr` (side play around the glass, 0.35 mm
now; don't go much below 0.2), `thr_clr` (thread clearance), and
`usb_plug_w` / `usb_plug_h` (USB window). Set `tail_angle` and `usb_angle` to
match which side your flex tail and cable leave from. To regenerate the STLs:

```bash
openscad -o case/sunton_2424s012_front.stl -D 'part="front"' case/sunton_2424s012_case.scad
openscad -o case/sunton_2424s012_back.stl  -D 'part="back"'  case/sunton_2424s012_case.scad
```

**Desk stand.** `case/sunton_2424s012_stand.scad` (and its `.stl`) is a wedge-shaped
dock: a flat base with a round cradle leaning back 15 degrees. The case drops in
plug-side first and is held by a friction fit (`fit_clr`, 0.25 mm). Cable notches on
both sides let the USB cable leave left or right. Print it base-down, no supports.
The MiSTer-Kun mascot is embossed on the front (`logo` = `"mascot"`, `"text"` or
`"none"`); the artwork is credited in `case/mister_kun_CREDITS.txt`. The owner
printed it and reports it works well.

![The Sunton board in the 3D-printed case, sitting in the matching desk stand (MiSTer-Kun embossed on the front), showing the WiFi "Connected" screen](case-stand-sunton.jpg)

Both the case and this stand print from the `.scad`/`.stl` files in `case/` — no
supports, standard FDM.

```bash
openscad -o case/sunton_2424s012_stand.stl case/sunton_2424s012_stand.scad
```
