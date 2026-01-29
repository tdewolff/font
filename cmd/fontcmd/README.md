# Fontcmd

## Info
Display font information.

```
Usage: fontcmd info [options] input

Arguments:
  input     Input file
```

## Draw
Draw font glyphs in the terminal or to an image file.

```
Usage: fontcmd draw [options] input

Options:
  -c, --char string    Literal character
  -g, --glyph uint     Glyph ID
  -h, --help           Help
  -i, --index int      Font index for font collections
  -o, --output string  Output filename, supports jpg, png, gif, and tiff
      --ppem=40 uint   Pixels per em-square
      --ratio float    Image width/height ratio
      --scale=4 int    Image scale
  -u, --unicode string Unicode codepoint
      --width int      Image width

Arguments:
  input     Input file
```

Example for the character A in DejaVuSans:
```
GlyphID: 36
Char: A (65)
Name: A
                                                                                  
                                                                                  
                                                                                  
                                                                                  
                                                                                  
                                                                                  
                                                                                  
                                                                                  
                                   ^i}nYYYYj>`.                                   
                                   ivd&$$$$oz[:                                   
                                 '"}p@$$$$$8aY<                                   
                                 ;1Y*$$$$$$$$b|I`                                 
                                 ~C#B$$$$$$$$#Q/I                                 
                               ^i/k$$&dd&$$$$@8O_.                                
                               !jZW%#L[{d$$$$$$ax_,                               
                             .'?mB@*X{;icb&$$$$&dvi                               
                             ,]va$@q[^.,-na$$$$$@p}"'                             
                             >Xh8&qni   .-Z%@$$$$*U);                             
                           ';)d$$hj~"    lt0M$$$$BMC~                             
                           I\L#B&0+.     `l|b$$$$$$kti^                           
                          .+0&B#C|I        <Uo%$$$$Wmj!                           
                         "~rh$$d):'        :}zo$$$$@Bm?'.                         
                         inp&8hz>          .`]wB@$$$$oc]:                         
                       .^[q@$au?,            !xwW$$$$8aX>                         
                       ;{X*@%m-'.            "<fh$$$$$$b);`                       
                       <J*%MOfl                ~LWB$$$$#L\I                       
                     ^!\b$$b\!^                ;(J*$$$$B&0+.                      
                     lfOM$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$hr+"                     
                   .'-Z%@$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$$&pui                     
                   ,?ua$@p1I:,,,,,,,,,,,,,,,,,,,,i{zo$$$$$@q[^.                   
                   >zk8Wqn!                      .'?mB@$$$$*Y1;                   
                 ',1d$$hj~"                        !jZW$$$$%#J<                   
                 I(C#B&0+.                         ^i/k$$$$$$k/!^                 
                 ~QWB#L|I                            ~C#B$$$$MOfl                 
               "<fh$$d);'                            ;1U*$$$$@%m-'.               
           I(nnXZa8@%hUnnnx);                  `;_/nnvXOM$$$$$$WmUvnn(I           
           _q$$$$$$$$$$$$$@m_                  ,_xa$$$$$$$$$$$$$$$$$$q_           
           l/zzzzzzzzzzzzzc\I                  `I?jzzzzzzzzzzzzzzzzzz/l
```

You can also export as rasterised image using `-o out.png`.

## Subset
Reduce the size of a font file by selecting a subset of the glyphs. Typically, fonts contain a significant portion of the total Unicode space, which can create large files especially when needed to be transferred over the wire and latency is an issue (loading websites, especially over cellular network). By removing all the glyphs of characters that are not used, the file size can be orders of maginitude smaller.

Example: the DejaVuSans.ttf file is 380 kB and contains 3528 glyphs. Only selecting only common ASCII characters used in the English language (78 glyphs) the file size becomes 17 kB (or 4.6% of the original). Saving as a WOFF2 file reduces the file size to 11 kB.

```
Usage: fontcmd subset [options] input

Options:
  -c, --char []string     List of literal characters to keep, eg. a-z.
  -e, --encoding string   Output encoding, either empty of base64.
  -f, --force             Force overwriting existing files.
  -g, --glyph []string    List of glyph IDs to keep, eg. 1-100.
      --glyph-name string New glyph name. Available variables: %i glyph ID, %n glyph name, %u glyph unicode in hexadecimal.
  -h, --help              Help
  -i, --index int         Index into font collection (used with TTC or OTC).
  -n, --name []string     List of glyph names to keep, eg. space.
  -o, --outputs []string  Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported). Can output multiple file.
  -q, --quiet             Suppress output except for errors.
  -r, --range []string    List of unicode categories or scripts to keep, eg. L (for Letters) or Latin (latin script). See
                          https://pkg.go.dev/unicode for all supported values.
  -t, --type string       Explicitly set output mimetype, eg. font/woff2.
  -u, --unicode []string  List of unicode IDs to keep, eg. f0fc-f0ff.

Arguments:
  input     Input font file.
```

### Examples
```
fontcmd subset -c'a-zA-Z0-9,.:;?!@#$%()" -' -c"'" --out dejavu\_subset.ttf DejavuSans.ttf
```

```
fontcmd subset -nenvelope,user,phone -ofa.woff fa-solid-900.ttf
```

## Merge
Merge the glyphs of two fonts. Unicode mappings must not overlap.

```
Usage: fontcmd merge [options] inputs...

Options:
  -e, --encoding string  Output encoding, either empty of base64.
  -f, --force            Force overwriting existing files.
  -h, --help             Help
  -o, --outputs []string Output font file (only TTF/OTF/WOFF2/TTC/OTC are supported). Can output multiple file.
  -q, --quiet            Suppress output except for errors.
      --rearrange-cmap   Rearrange glyph unicode mapping, assigning a sequential codepoint for each glyph in order starting at 33
                         (exclamation).
  -t, --type string      Explicitly set output mimetype, eg. font/woff2.

Arguments:
  inputs    Input font files.
```

## CSS
Export the corresponding CSS file to reference the characters using `<i class=".prefix-glyphname"></i>`, eg. for FontAwesome or similar fonts.

```
Usage: fontcmd css [options] input

Options:
  -a, --append              Append to the output file instead of overwriting.
  -f, --force               Force overwriting existing files.
  -h, --help                Help
  -i, --index int           Index into font collection (used with TTC or OTC).
  -o, --output string       CSS output file name.
  -q, --quiet               Suppress output except for errors.
      --selector=.%n string Glyph name selector to use for CSS. Available variables: %i glyph ID, %n glyph name, %u glyph unicode in
                            hexadecimal.

Arguments:
  input     Input font file.
```
