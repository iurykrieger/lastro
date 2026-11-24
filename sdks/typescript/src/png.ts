/**
 * PNG decoding and the share of pixels changed between two screenshots (Node only: it uses node:zlib).
 *
 * Supports what WebDriver and Appium screenshots use: 8-bit, non-interlaced, grayscale, RGB, palette, grayscale with
 * alpha and RGBA. Every pixel is compared as RGBA; images of different sizes differ in every pixel.
 */
import { inflateSync } from "node:zlib";

export interface Image {
  width: number;
  height: number;
  /** RGBA, row by row. */
  rgba: Uint8Array;
}

export function decodePng(bytes: Uint8Array): Image {
  const b = Buffer.from(bytes);
  const sig = [137, 80, 78, 71, 13, 10, 26, 10];
  if (b.length < 8 || sig.some((x, i) => b[i] !== x)) throw new Error("not a PNG");
  let width = 0;
  let height = 0;
  let depth = 0;
  let color = 0;
  let interlace = 0;
  let palette: Buffer | undefined;
  let trns: Buffer | undefined;
  const idat: Buffer[] = [];
  for (let o = 8; o < b.length; ) {
    const len = b.readUInt32BE(o);
    const type = b.toString("latin1", o + 4, o + 8);
    const data = b.subarray(o + 8, o + 8 + len);
    if (type === "IHDR") {
      width = data.readUInt32BE(0);
      height = data.readUInt32BE(4);
      depth = data[8]!;
      color = data[9]!;
      interlace = data[12]!;
    } else if (type === "PLTE") palette = data;
    else if (type === "tRNS") trns = data;
    else if (type === "IDAT") idat.push(data);
    else if (type === "IEND") break;
    o += 12 + len;
  }
  if (depth !== 8 || interlace !== 0) throw new Error(`unsupported PNG: bit depth ${depth}, interlace ${interlace}`);
  const channels = { 0: 1, 2: 3, 3: 1, 4: 2, 6: 4 }[color as 0 | 2 | 3 | 4 | 6];
  if (!channels) throw new Error(`unsupported PNG color type ${color}`);
  const raw = inflateSync(Buffer.concat(idat));
  const stride = width * channels;
  const px = new Uint8Array(height * stride);
  for (let y = 0; y < height; y++) {
    const filter = raw[y * (stride + 1)]!;
    const line = raw.subarray(y * (stride + 1) + 1, (y + 1) * (stride + 1));
    for (let x = 0; x < stride; x++) {
      const a = x >= channels ? px[y * stride + x - channels]! : 0;
      const up = y > 0 ? px[(y - 1) * stride + x]! : 0;
      const c = x >= channels && y > 0 ? px[(y - 1) * stride + x - channels]! : 0;
      const v = line[x]!;
      let out: number;
      if (filter === 0) out = v;
      else if (filter === 1) out = v + a;
      else if (filter === 2) out = v + up;
      else if (filter === 3) out = v + ((a + up) >> 1);
      else if (filter === 4) {
        const p = a + up - c;
        const pa = Math.abs(p - a);
        const pb = Math.abs(p - up);
        const pc = Math.abs(p - c);
        out = v + (pa <= pb && pa <= pc ? a : pb <= pc ? up : c);
      } else throw new Error(`bad PNG filter ${filter}`);
      px[y * stride + x] = out & 0xff;
    }
  }
  const rgba = new Uint8Array(width * height * 4);
  for (let i = 0; i < width * height; i++) {
    const s = px.subarray(i * channels, (i + 1) * channels);
    let r: number, g: number, bl: number, al = 255;
    if (color === 0) r = g = bl = s[0]!;
    else if (color === 2) [r, g, bl] = [s[0]!, s[1]!, s[2]!];
    else if (color === 3) {
      const k = s[0]!;
      [r, g, bl] = [palette![k * 3]!, palette![k * 3 + 1]!, palette![k * 3 + 2]!];
      if (trns && k < trns.length) al = trns[k]!;
    } else if (color === 4) [r, g, bl, al] = [s[0]!, s[0]!, s[0]!, s[1]!];
    else [r, g, bl, al] = [s[0]!, s[1]!, s[2]!, s[3]!];
    rgba.set([r, g, bl, al], i * 4);
  }
  return { width, height, rgba };
}

/** The share of pixels that differ, from 0 to 1. Images of different sizes differ entirely. */
export function pixelShare(a: Uint8Array, b: Uint8Array): number {
  const x = decodePng(a);
  const y = decodePng(b);
  if (x.width !== y.width || x.height !== y.height) return 1;
  const n = x.width * x.height;
  if (!n) return 0;
  let changed = 0;
  for (let i = 0; i < n; i++) {
    const k = i * 4;
    if (x.rgba[k] !== y.rgba[k] || x.rgba[k + 1] !== y.rgba[k + 1] || x.rgba[k + 2] !== y.rgba[k + 2] || x.rgba[k + 3] !== y.rgba[k + 3]) changed++;
  }
  return changed / n;
}
