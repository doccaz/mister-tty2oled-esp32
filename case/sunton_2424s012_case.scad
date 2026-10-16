// Compact screw-together case for the Sunton ESP32-2424S012C (1.28" round GC9A01
// with the ESP32-C3 on the back). Parametric OpenSCAD; GPL-3.0-or-later like the project.
//
// Two parts:
//   "front" - the cup: bezel face with the display window, a bore that centres the
//             glass (six small guide pads), the walls, and the USB-C opening. Print it
//             face-down.
//   "back"  - a threaded plug: a flat disc (same diameter as the cup, so the seam is a
//             hairline groove) with a hollow threaded skirt. Print it flat, thread up.
// The plug screws into the cup from behind (right-hand thread, 2 turns). The USB-C
// opening is cut out of the cup wall below the thread's top, so no part of the thread
// ring blocks the plug: the skirt has a wide matching cut, because the exact angle
// it stops at depends on print tolerance.
//
// Dimensions: display glass, FPC tail from the vendor drawing; the USB plug size from
// your photos; thickness/back height are still estimates - check with calipers.
//
// Optional: with back_pedestal = true the plug has a flat disc under the board's tallest
// parts, so the board can only sag back by ped_gap (a thin foam pad then holds it forward).
//
// part = "front" | "back" | "both" (side by side) | "assembly" (cutaway preview)

part = "both";

/* ---- Board ---- */
disp_d       = 37.0;  // display glass diameter (vendor drawing)
pcb_d        = 38.6;  // room for the PCB and its back-side parts (any part past the glass?)
tail_w       = 20.0;  // FPC tail slot width (vendor 14.23, but it is wider where it leaves the glass and the fold bulges)
tail_fold    = 1.4;   // extra radial room for the fold behind the PCB
tail_len     = 1.5;   // tail sticks out past the glass (38.50 - 37.00)
tail_angle   = 270;   // direction of the tail (deg, 0 = +X, CCW from the front)
pcb_t        = 1.6;   // PCB thickness
glass_h      = 2.4;   // glass height above the PCB front face
view_d       = 32.4;  // visible active-area diameter
back_h       = 4.6;   // tallest part behind the PCB (C3 module, connectors)

/* ---- USB-C ---- */
usb_angle    = 270;   // where the receptacle sits around the edge (deg)
usb_open_to_rim = false; // true: notch open to the rim (no thin band to snap, but a taller gap)
usb_z        = 1.7;   // receptacle centre below the PCB back face
usb_plug_w   = 12.0;  // opening width: the cable's plug overmold (11.5) + play, not the port
usb_plug_h   = 7.0;   // opening height: overmold (6.5) + play

/* ---- Case ---- */
clr        = 0.3;     // radial clearance around the PCB pocket
glass_clr  = 0.35;    // radial clearance between the glass and the guide pads
pads       = 6;       // number of glass guide pads (the gaps between them clear the FPC fold)
pad_deg    = 16;      // angular width of each pad
pad_phase  = 30;      // angle of the first pad
lip        = 0.8;     // how far the bezel overlaps the glass edge on the front
face_t     = 1.6;     // bezel face thickness
fil        = 2.0;     // front edge rounding
back_fil   = 3.5;     // back edge rounding of the plug
win_ch     = 0.8;     // chamfer on the window edge
float_gap  = 1.4;     // free space between the PCB's rear parts and the plug floor... (see z_p)
glass_gap  = 0.3;     // free space between the glass and the bezel lip
flange_t   = 1.8;     // plug disc thickness
$fn        = 96;

/* ---- Optional pedestal on the plug (limits how far the board can sag back) ---- */
back_pedestal = true; // flat disc on the plug floor, just below the board's tallest parts
ped_gap       = 0.5;  // clearance left between the disc and the tallest part (back_h)

/* ---- Thread ---- */
pitch      = 2.0;     // axial advance per turn
turns      = 2;       // threaded length = pitch * turns
e          = 0.35;    // thread depth (peak-to-peak is 2*e); sine profile
skirt_wall = 1.1;     // plug skirt wall thickness (at the thread root)
thr_clr    = 0.35;    // radial clearance between thread and bore (raise if tight)
cup_wall   = 1.2;     // cup wall outside the thread (at the thread peak)

/* ---- Derived (assembly coordinates: plug bottom at z = 0, display faces +Z) ---- */
R_in    = pcb_d/2 + clr;
R_g     = disp_d/2 + glass_clr;
thread_h = pitch * turns;
r_m     = R_in + skirt_wall + e;             // male thread centre radius
r_f     = r_m + thr_clr;                     // female bore centre radius
R_out   = r_f + e + cup_wall;
z_p     = flange_t + float_gap + back_h;     // PCB back face
z_g0    = z_p - 0.6;                         // centring bore starts here
lip_z   = z_p + pcb_t + glass_h + glass_gap; // underside of the bezel lip
cup_top = lip_z + face_t;
z_u     = z_p + glass_gap - usb_z;           // USB receptacle axis, display pushed against the lip (foam)
H       = usb_plug_h/2;

echo(outer_diameter = 2*R_out, total_height = cup_top);

module helix(r, h) {
  // eccentric circle, twisted: a sine-profile, right-hand thread of pitch `pitch`
  linear_extrude(height = h, twist = -360*h/pitch, slices = max(8, ceil(h/pitch*40)), convexity = 6)
    translate([e, 0]) circle(r = r, $fn = 72);
}

// z_lo: bottom of the opening. The cup passes flange_t so the window is open to its
// rim - a sliver of wall under the opening is paper thin and snaps off the print bed.
module usb_cut(w, r0, z_lo = z_u - H) {
  rotate([0, 0, usb_angle])
    translate([r0, -w/2, z_lo]) cube([R_out + 3 - r0, w, z_u + H - z_lo]);
}

module cup() {
  difference() {
    // outer body, rim chamfer at the bottom, rounded front edge
    translate([0, 0, flange_t]) rotate_extrude() hull() {
      h = cup_top - flange_t;
      square([0.01, h]);
      translate([R_out - fil, h - fil]) circle(fil);
      polygon([[0, 0], [R_out - 0.4, 0], [R_out, 0.4], [R_out, 0.5]]);
    }
    // internal thread, a little longer than the plug's so nothing bottoms out
    translate([0, 0, flange_t - 0.01]) helix(r_f, thread_h + 0.5);
    // PCB / back-parts pocket
    translate([0, 0, flange_t - 0.01]) cylinder(r = R_in, h = z_g0 - flange_t + 0.01);
    // glass area: full pocket radius, minus a few guide pads that centre the glass.
    // (A full bore ring pressed on the FPC tail's fold and tipped the board.)
    translate([0, 0, z_g0 - 0.01]) difference() {
      cylinder(r = R_in, h = lip_z - z_g0 + 0.01);
      for (i = [0 : pads - 1]) rotate([0, 0, pad_phase + i*360/pads - pad_deg/2])
        translate([0, 0, -0.1]) rotate_extrude(angle = pad_deg)
          translate([R_g, 0]) square([R_in + 0.5 - R_g, lip_z - z_g0 + 0.3]);
    }
    // FPC tail slot through the pad ring, and relief for the fold in the pocket below it
    // Clipped to a cylinder so the rectangle's corners can't break through the wall.
    intersection() {
      translate([0, 0, flange_t]) cylinder(r = R_out - cup_wall, h = lip_z);
      rotate([0, 0, tail_angle]) {
        translate([0, -(tail_w + 0.6)/2, z_g0 - 0.01])
          cube([disp_d/2 + tail_len + 0.6, tail_w + 0.6, lip_z - z_g0 + 0.01]);
        translate([0, -(tail_w + 0.6)/2, flange_t + thread_h + 0.5])
          cube([R_in + tail_fold, tail_w + 0.6, lip_z - (flange_t + thread_h + 0.5)]);
      }
    }
    // viewing window with a chamfered outer edge
    translate([0, 0, lip_z - 0.01]) cylinder(d = view_d + 2*lip, h = face_t + 0.1);
    translate([0, 0, cup_top - win_ch]) cylinder(d1 = view_d + 2*lip, d2 = view_d + 2*lip + 2*win_ch, h = win_ch + 0.01);
    // USB-C plug opening, reaching in to the glass bore so the overmold clears
    usb_cut(usb_plug_w, disp_d/2 - 1, usb_open_to_rim ? flange_t - 0.01 : z_u - H);
  }
}

// A full disc (not posts): the plug turns about 2 turns while it rises, so anything
// narrower would sweep across the board's parts. A disc below the tallest part (back_h)
// can't touch anything, and the board can then only drop back by ped_gap before the
// tallest parts rest on it - so the foam only has to take up that small gap.
module pedestal() {
  translate([0, 0, flange_t - 0.01]) cylinder(r = R_in - 0.6, h = z_p - back_h - ped_gap - flange_t + 0.01);
}

module plug() {
  union() {
  difference() {
    union() {
      // flange: same diameter as the cup, rounded underneath, chamfered where it meets it
      rotate_extrude() intersection() {
        hull() {
          square([0.01, flange_t]);
          translate([R_out - back_fil, back_fil]) circle(back_fil);
        }
        polygon([[0, 0], [R_out, 0], [R_out, flange_t - 0.4], [R_out - 0.4, flange_t], [0, flange_t]]);
      }
      // hollow threaded skirt
      translate([0, 0, flange_t - 0.01]) difference() {
        helix(r_m, thread_h);
        translate([0, 0, -0.1]) cylinder(r = R_in, h = thread_h + 0.2);
      }
    }
    // wide cut in the skirt, behind the cup's USB opening (the stop angle isn't exact)
    intersection() {
      usb_cut(usb_plug_w + 14, R_in - 1);
      translate([-R_out - 5, -R_out - 5, flange_t]) cube([2*R_out + 10, 2*R_out + 10, thread_h + 1]);
    }
    // coin slot on the underside
    translate([0, 0, -0.01]) linear_extrude(1.0) square([R_out*1.2, 2.0], center = true);
  }
  if (back_pedestal) pedestal();
  }
}

if (part == "front") rotate([180, 0, 0]) translate([0, 0, -cup_top]) cup();
else if (part == "back") plug();
else if (part == "assembly") {
  difference() {
    union() { cup(); plug(); }
    translate([0, -50, -1]) cube([100, 100, 100]); // cutaway: keeps x < 0 ... see camera
  }
} else {
  translate([-R_out - 3, 0, 0]) rotate([180, 0, 0]) translate([0, 0, -cup_top]) cup();
  translate([ R_out + 3, 0, 0]) plug();
}
