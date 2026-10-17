// Desk stand for the Sunton ESP32-2424S012C case (sunton_2424s012_case.scad).
// Parametric OpenSCAD; GPL-3.0-or-later like the project.
//
// A wedge-shaped dock: a flat base with a round cradle leaning back `lean` degrees. The
// case drops into the cradle plug-side first (bezel facing out) and is held by a snug
// friction fit. A notch in the cradle wall lets the USB cable leave sideways: turn the
// case in the cradle until its USB window faces the notch. Print it base-down, no
// supports (the back leans only `lean` degrees off vertical).
//
// case_d must match the case's outer diameter (the case file prints it as
// "outer_diameter" when rendered). Check the fit on your printer and adjust fit_clr.

case_d     = 45.5;   // outer diameter of the case (sunton_2424s012_case.scad)
fit_clr    = 0.25;   // radial clearance between case and cradle (raise if tight)
lean       = 15;     // how far the display leans back from vertical (deg)
plate_t    = 2.5;    // cradle floor thickness
ring_h     = 7.0;    // how deep the case sits in the cradle (case is 13.7 tall)
ring_w     = 2.5;    // cradle wall thickness
exit_angles = [0, 180]; // cable notches, in the display plane (0 = viewer's right, 180 = left); [] = none
notch_w    = 16.0;   // cable notch width (the plug is 11.5 wide, plus play)
stem_gap   = 6.0;    // space between the base and the lowest point of the cradle (the hull fills it)

base_x     = 36;     // base half-width
base_front = 40;     // base reach in front of the cradle centre
base_back  = 42;     // base reach behind it (the display leans back, so more here)
base_t     = 4.0;    // base thickness
base_r     = 9;      // base corner radius
$fn        = 96;

Rc  = case_d/2 + fit_clr;       // cradle bore radius
Ro  = Rc + ring_w;              // cradle outer radius
cz  = base_t + stem_gap + Ro*cos(lean);   // height of the cradle centre

logo       = "mascot"; // embossed on the front slope: "mascot" (MiSTer-Kun), "text" or "none"
logo_text  = "MiSTer";  // used when logo = "text"
logo_font  = "Liberation Sans:style=Bold";
logo_w     = 17.0;   // mascot width (text: letter height)
logo_h     = 1.0;    // emboss height
logo_s     = 9.0;    // distance up the slope from the base's front edge to the logo centre

include <mister_kun_logo.scad>   // MiSTer-Kun mascot (credits: mister_kun_CREDITS.txt)

// Frame: display faces -Y (the viewer is at -Y), up is +Z, leaning back toward +Y.
module cradle_frame() {
  translate([0, 0, cz]) rotate([-lean, 0, 0]) rotate([90, 0, 0]) children();
}

module base() {
  hull() for (sx = [-1, 1], y = [-base_front + base_r, base_back - base_r])
    translate([sx*(base_x - base_r), y, 0]) cylinder(r = base_r, h = base_t);
}

module stand() {
  difference() {
    hull() {
      base();
      cradle_frame() cylinder(r = Ro, h = plate_t + ring_h);
    }
    cradle_frame() {
      // bore for the case
      translate([0, 0, plate_t]) cylinder(r = Rc, h = ring_h + 50);
      // cable notches through the wall, open at the top
      for (a = exit_angles) rotate([0, 0, a])
        translate([0, -notch_w/2, plate_t]) cube([Ro + 30, notch_w, ring_h + 50]);
    }
  }
}

// cradle-frame point -> world (same transform as cradle_frame())
function cradle_pt(p) =
  let(y1 = -p[2], z1 = p[1])
  [p[0], y1*cos(lean) + z1*sin(lean), -y1*sin(lean) + z1*cos(lean) + cz];

// The hull's front face is the plane from the base's front edge up to the top of the
// cradle's front rim; the logo is embossed on it, centred, reading upward.
module logo() {
  A = [0, -base_front, base_t];
  T = cradle_pt([0, Ro, plate_t + ring_h]);
  d = T - A;
  beta = atan2(d[2], d[1]);
  len = norm([d[1], d[2]]);
  P = A + [0, d[1], d[2]]*(logo_s/len);
  translate(P) rotate([beta, 0, 0]) translate([0, 0, -0.4])
    linear_extrude(height = logo_h + 0.4) {
      if (logo == "mascot") scale(logo_w) kun_logo2d();
      else text(logo_text, size = logo_w*0.4, font = logo_font, halign = "center", valign = "center");
    }
}

union() { stand(); if (logo != "none") logo(); }
