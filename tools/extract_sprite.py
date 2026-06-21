"""
extract_sprite.py
Background removal for hand-drawn / outlined sprites photographed or scanned on
near-neutral paper. Outputs a tight-cropped transparent PNG.

WHEN THIS APPROACH WORKS
  - Subject has a dark (near-black) outline AND/OR is noticeably more saturated
    than the background.
  - Background is roughly neutral (low chroma): white/grey paper, even under
    uneven lighting or a soft gradient/shadow.
  This is the case where a naive "remove white" fails (the subject may contain
  light areas too) but chroma keying is clean.

WHEN IT DOES NOT WORK (bail out / use a different method)
  - Subject is itself low-chroma (greyscale art, white-on-white, pencil) -> there
    is no chroma signal separating it from paper. No tuning fixes this; you need
    the layered source or a hand mask.
  - Background is colored/busy rather than neutral paper.

ALGORITHM (each step exists for a reason; do not drop one without checking)
  1. Chroma key:  bg = (saturation < SAT) AND (value > VAL_GUARD)
       - low saturation  -> neutral paper (and the black outline, which is also
         neutral) both qualify on chroma alone...
       - ...so the VAL_GUARD brightness floor RE-CLAIMS the dark outline as
         foreground. Paper is bright; the outline is dark. This is the whole
         trick that keeps the outline intact.
  2. Outer background = flood fill the bg mask inward from the image border.
     Only background reachable from outside is removed; light areas sealed inside
     the outline (eye glints, teeth) are never touched.
  3. Enclosed holes: bg-colored pockets NOT reachable from the border (e.g. the
     gap inside a curled tail). Large ones (> HOLE_AREA) are punched back to
     transparent so they read as see-through; small ones (glints) stay opaque.
  4. Component filter: keep only connected foreground blobs > MIN_AREA. Kills
     paper speckle and stray marker smudges in the corners.
  5. Close + erode: a 2px erosion pulls the matte just INSIDE the black outline,
     removing the anti-aliased paper fringe that otherwise shows as a light halo
     on dark in-engine backgrounds. Costs a hair of outline thickness (invisible
     at sprite scale). Drop to ERODE=1 and decontaminate instead if you need the
     full line weight.
  6. Feather alpha 1px, crop to bbox + PAD.

TUNING
  Run with --inspect first on a new source to read its saturation/value
  distribution and corner samples, then set SAT / VAL_GUARD from that. Defaults
  below were derived from a marker drawing on grey paper (paper S~4-22 V~100-123,
  subject S~150+).

DEPS:  pip install opencv-python-headless numpy
USAGE:
  python extract_sprite.py in.png -o out.png
  python extract_sprite.py in.png --inspect          # print thresholds to set
  python extract_sprite.py sheet/*.png --batch out_dir/
"""

import argparse
import os
import cv2
import numpy as np


def inspect(img):
    """Print the stats you need to choose SAT / VAL_GUARD for a new source."""
    hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
    S, V = hsv[..., 1], hsv[..., 2]
    h, w = S.shape
    print(f"  size {w}x{h}")
    for y, x, lbl in [(5, 5, "TL"), (5, w - 5, "TR"),
                      (h - 5, 5, "BL"), (h - 5, w - 5, "BR")]:
        print(f"  corner {lbl}: S={int(S[y, x])} V={int(V[y, x])}")
    sp = [int(np.percentile(S, p)) for p in (50, 75, 90, 95, 99)]
    vp = [int(np.percentile(V, p)) for p in (1, 5, 10, 25, 50)]
    print(f"  S pct [50,75,90,95,99] = {sp}   (bg low, subject high)")
    print(f"  V pct [1,5,10,25,50]   = {vp}   (outline low, paper high)")
    print("  -> set SAT between the bg S and subject S; "
          "set VAL_GUARD above the outline V, below paper V")


def extract(img, sat=50, val_guard=78, hole_area=350,
            min_area=1500, erode_iters=2, pad=8):
    hsv = cv2.cvtColor(img, cv2.COLOR_BGR2HSV)
    S = hsv[..., 1].astype(int)
    V = hsv[..., 2].astype(int)
    h, w = S.shape

    # 1. chroma key with brightness guard (keeps dark outline as foreground)
    bg = ((S < sat) & (V > val_guard)).astype(np.uint8)

    # 2. outer background: flood the bg mask from the border
    ff = bg.copy()
    mask = np.zeros((h + 2, w + 2), np.uint8)
    for x in range(0, w, 15):
        for y in (0, h - 1):
            if ff[y, x]:
                cv2.floodFill(ff, mask, (x, y), 2)
    for y in range(0, h, 15):
        for x in (0, w - 1):
            if ff[y, x]:
                cv2.floodFill(ff, mask, (x, y), 2)
    outer_bg = ff == 2

    # everything not reachable from outside is foreground (incl. enclosed holes)
    fg = ~outer_bg

    # 3. punch large enclosed bg pockets back to transparent (e.g. tail loop)
    enclosed = bg.astype(bool) & ~outer_bg
    n, lbl, stats, _ = cv2.connectedComponentsWithStats(
        enclosed.astype(np.uint8), 8)
    for i in range(1, n):
        if stats[i, cv2.CC_STAT_AREA] > hole_area:
            fg[lbl == i] = False

    # 4. keep only sizeable foreground components (drop speckle / smudges)
    n, lbl, stats, _ = cv2.connectedComponentsWithStats(
        fg.astype(np.uint8), 8)
    keep = np.zeros((h, w), np.uint8)
    for i in range(1, n):
        if stats[i, cv2.CC_STAT_AREA] > min_area:
            keep[lbl == i] = 1

    # 5. close gaps, then erode just inside the outline to drop the light fringe
    keep = cv2.morphologyEx(keep, cv2.MORPH_CLOSE, np.ones((3, 3), np.uint8))
    if erode_iters > 0:
        keep = cv2.erode(keep, np.ones((3, 3), np.uint8), iterations=erode_iters)

    # 6. feather alpha, crop to bbox
    alpha = cv2.GaussianBlur((keep * 255).astype(np.uint8), (3, 3), 0)
    out = cv2.cvtColor(img, cv2.COLOR_BGR2BGRA)
    out[..., 3] = alpha

    ys, xs = np.where(alpha > 10)
    if len(ys) == 0:
        raise ValueError("nothing kept -- thresholds likely wrong; run --inspect")
    y0, y1 = max(ys.min() - pad, 0), min(ys.max() + pad, h)
    x0, x1 = max(xs.min() - pad, 0), min(xs.max() + pad, w)
    return out[y0:y1, x0:x1]


def main():
    ap = argparse.ArgumentParser(description="Chroma-key sprite background removal")
    ap.add_argument("inputs", nargs="+")
    ap.add_argument("-o", "--output", default=None)
    ap.add_argument("--batch", default=None, help="output dir for multiple inputs")
    ap.add_argument("--inspect", action="store_true", help="print stats and exit")
    ap.add_argument("--sat", type=int, default=50)
    ap.add_argument("--val-guard", type=int, default=78)
    ap.add_argument("--hole-area", type=int, default=350)
    ap.add_argument("--min-area", type=int, default=1500)
    ap.add_argument("--erode", type=int, default=2)
    ap.add_argument("--pad", type=int, default=8)
    a = ap.parse_args()

    for path in a.inputs:
        img = cv2.imread(path)
        if img is None:
            print(f"skip (unreadable): {path}")
            continue
        if a.inspect:
            print(path)
            inspect(img)
            continue
        cut = extract(img, a.sat, a.val_guard, a.hole_area,
                      a.min_area, a.erode, a.pad)
        if a.batch:
            os.makedirs(a.batch, exist_ok=True)
            dst = os.path.join(
                a.batch, os.path.splitext(os.path.basename(path))[0] + "_cut.png")
        else:
            dst = a.output or os.path.splitext(path)[0] + "_cut.png"
        cv2.imwrite(dst, cut)
        print(f"wrote {dst}  {cut.shape[1]}x{cut.shape[0]}")


if __name__ == "__main__":
    main()
