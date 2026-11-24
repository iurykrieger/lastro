using System.Buffers.Binary;
using System.IO.Compression;
using System.Text;

namespace Lastro;

/// <summary>A minimal PNG reader (8-bit, not interlaced), as the TS SDK's png.ts: enough to compare screenshots.</summary>
public static class Png
{
    public sealed record Image(int Width, int Height, byte[] Rgba);

    /// <summary>The image, or null when the bytes are not a PNG this reader supports.</summary>
    public static Image? Decode(byte[] b)
    {
        byte[] sig = [137, 80, 78, 71, 13, 10, 26, 10];
        if (b.Length < 8 || !b.AsSpan(0, 8).SequenceEqual(sig)) return null;
        int width = 0, height = 0, depth = 0, color = 0, interlace = 0;
        byte[]? palette = null, trns = null;
        using var idat = new MemoryStream();
        for (var o = 8; o + 8 <= b.Length;)
        {
            var len = (int)BinaryPrimitives.ReadUInt32BigEndian(b.AsSpan(o));
            var type = Encoding.Latin1.GetString(b, o + 4, 4);
            var data = b.AsSpan(o + 8, len);
            if (type == "IHDR")
            {
                width = (int)BinaryPrimitives.ReadUInt32BigEndian(data);
                height = (int)BinaryPrimitives.ReadUInt32BigEndian(data[4..]);
                (depth, color, interlace) = (data[8], data[9], data[12]);
            }
            else if (type == "PLTE") palette = data.ToArray();
            else if (type == "tRNS") trns = data.ToArray();
            else if (type == "IDAT") idat.Write(data);
            else if (type == "IEND") break;
            o += 12 + len;
        }
        int channels = color switch { 0 => 1, 2 => 3, 3 => 1, 4 => 2, 6 => 4, _ => 0 };
        if (depth != 8 || interlace != 0 || channels == 0) return null;
        idat.Position = 0;
        using var z = new ZLibStream(idat, CompressionMode.Decompress);
        using var rawStream = new MemoryStream();
        z.CopyTo(rawStream);
        var raw = rawStream.ToArray();
        var stride = width * channels;
        var px = new byte[height * stride];
        for (var y = 0; y < height; y++)
        {
            var filter = raw[y * (stride + 1)];
            var line = y * (stride + 1) + 1;
            for (var x = 0; x < stride; x++)
            {
                int a = x >= channels ? px[y * stride + x - channels] : 0;
                int up = y > 0 ? px[(y - 1) * stride + x] : 0;
                int c = x >= channels && y > 0 ? px[(y - 1) * stride + x - channels] : 0;
                int v = raw[line + x];
                int outv = filter switch
                {
                    0 => v,
                    1 => v + a,
                    2 => v + up,
                    3 => v + ((a + up) >> 1),
                    4 => v + Paeth(a, up, c),
                    _ => -1,
                };
                if (outv < 0) return null;
                px[y * stride + x] = (byte)(outv & 0xff);
            }
        }
        var rgba = new byte[width * height * 4];
        for (var i = 0; i < width * height; i++)
        {
            var s = i * channels;
            byte r, g, bl, al = 255;
            switch (color)
            {
                case 0: r = g = bl = px[s]; break;
                case 2: (r, g, bl) = (px[s], px[s + 1], px[s + 2]); break;
                case 3:
                    var k = px[s];
                    (r, g, bl) = (palette![k * 3], palette[k * 3 + 1], palette[k * 3 + 2]);
                    if (trns is not null && k < trns.Length) al = trns[k];
                    break;
                case 4: (r, g, bl, al) = (px[s], px[s], px[s], px[s + 1]); break;
                default: (r, g, bl, al) = (px[s], px[s + 1], px[s + 2], px[s + 3]); break;
            }
            (rgba[i * 4], rgba[i * 4 + 1], rgba[i * 4 + 2], rgba[i * 4 + 3]) = (r, g, bl, al);
        }
        return new Image(width, height, rgba);
    }

    private static int Paeth(int a, int b, int c)
    {
        var p = a + b - c;
        var (pa, pb, pc) = (Math.Abs(p - a), Math.Abs(p - b), Math.Abs(p - c));
        return pa <= pb && pa <= pc ? a : pb <= pc ? b : c;
    }
}
