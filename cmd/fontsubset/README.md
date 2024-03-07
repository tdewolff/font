# Fontsubset
Reduce the size of a font file by selecting a subset of the glyphs. Typically, fonts contain a significant portion of the total Unicode space, which can create large files especially when needed to be transferred over the wire and latency is an issue (loading websites, especially over cellular network). By removing all the glyphs of characters that are not used, the file size can be orders of maginitude smaller.

Example: the DejaVuSans.ttf file is 380 kB and contains 3528 glyphs. Only selecting only common ASCII characters used in the English language (78 glyphs) the file size becomes 17 kB (or 4.6% of the original). Saving as a WOFF2 file reduces the file size to 11 kB.

Additionally, you can export the corresponding CSS file to reference the characters using `<i class=".prefix-glyphname"></i>`, eg. for FontAwesome or similar fonts. You can also export the font as a base64 encoded string to incorporate in a HTML directly (and save an additional HTTP request).


## Features
- Load `.ttf`, `.otf`, `.ttc`, `.otc`, `.woff`, `.woff2`, `.eot` file formats
- Save as `.ttf`, `.otf`, `.ttc`, `.otc`, `.woff2` file formats (but cannot convert between TrueType and CFF glyph outlines)
- Save font as a base64 encoded string
- Select glyphs using literal characters, glyph IDs, glyph names, unicode codepoints, or unicode ranges
- Export CSS file that references glyph names using CSS class names


## Command line options
```
Usage: fontsubset [options] input

Options:
  -c, --char []string       List of literal characters to keep, eg. a-z.
      --css.output string   CSS output file name.
      --css.append          Append to the output file instead of overwriting.
      --css.encoding string Output encoding, either empty of base64.
      --css.prefix string   Glyph name prefix to use for CSS classes, eg. 'fa-'.
  -e, --encoding string     Output encoding, either empty of base64.
  -f, --force               Force overwriting existing files.
  -g, --glyph []string      List of glyph IDs to keep, eg. 1-100.
  -h, --help                Help
      --index int           Index into font collection (used with TTC or OTC).
  -n, --name []string       List of glyph names to keep, eg. space.
  -o, --output string       Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported).
  -q, --quiet               Suppress output except for errors.
  -r, --range []string      List of unicode categories or scripts to keep, eg. L (for Letters) or Latin
                            (latin script). See https://pkg.go.dev/unicode for all supported values.
  -u, --unicode []string    List of unicode IDs to keep, eg. f0fc-f0ff.

Arguments:
  input     Input font file.


## Example
```
fontsubset -c'a-zA-Z0-9,.:;?!@#$%()" -' -c"'" --out dejavu\_subset.ttf DejavuSans.ttf
```

```
fontsubset -nenvelope,user,phone -ofa.woff fa-solid-900.ttf
```
