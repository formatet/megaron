"""Gemensam bildhjälp för formark-verktygen — inga beroenden utöver ImageMagick.

numpy/PIL finns inte på en ren Arch-python (PEP 668 blockerar `pip install --user`),
och riggen ska gå att köra på vilken maskin som helst. ImageMagick skriver binär PGM
till stdout; att läsa den är tio rader och ger full pixelåtkomst.
"""
import subprocess


def load_gray(path, *, ops=()):
    """(bredd, höjd, bytes) i 8-bitars gråskala. `ops` = extra magick-argument
    (skalning, tröskling) som körs före utläsningen."""
    cmd = ["magick", str(path)] + list(ops)
    cmd += ["-colorspace", "Gray", "-depth", "8", "pgm:-"]
    raw = subprocess.run(cmd, capture_output=True, check=True).stdout
    pos, fields = 0, []
    while len(fields) < 4:
        while raw[pos:pos + 1].isspace():
            pos += 1
        if raw[pos:pos + 1] == b"#":
            while raw[pos:pos + 1] not in (b"\n", b""):
                pos += 1
            continue
        start = pos
        while not raw[pos:pos + 1].isspace():
            pos += 1
        fields.append(raw[start:pos])
    pos += 1  # exakt ett blanktecken mellan maxval och data
    w, h = int(fields[1]), int(fields[2])
    return w, h, raw[pos:pos + w * h]


def otsu(px):
    """Otsus tröskel. Arken har olika bläck — sigillen är svart tusch, flaggorna
    bleka konturer. En fast tröskel som passar det ena raderar det andra helt
    (50 % gav 496 identiska flaggpar, alltså tomma raster, inte ett fynd)."""
    hist = [0] * 256
    for b in px:
        hist[b] += 1
    total = len(px)
    sum_all = sum(i * hist[i] for i in range(256))
    sum_b = w_b = 0
    best, best_t = -1.0, 128
    for t in range(256):
        w_b += hist[t]
        if w_b == 0:
            continue
        w_f = total - w_b
        if w_f == 0:
            break
        sum_b += t * hist[t]
        var = w_b * w_f * ((sum_b / w_b) - ((sum_all - sum_b) / w_f)) ** 2
        if var > best:
            best, best_t = var, t
    return best_t


def ink_mask(w, h, px, threshold=None):
    """1 = bläck. Tröskeln är Otsus om ingen anges."""
    t = otsu(px) if threshold is None else threshold
    return [1 if px[y * w + x] <= t else 0 for y in range(h) for x in range(w)]


def fill_silhouette(w, h, mask):
    """Fyll en konturform till massiv siluett, radvis mellan yttersta bläcket.

    Flaggarket är ritat som tunna konturer, och en kontur är en ritkonvention —
    inte formen. Den försvinner dessutom helt vid nedskalning, så en omätt
    konturform ger tomma raster (det gav 496 "identiska" flaggpar innan detta).

    ⚠ **Radfyllning, inte flödesfyllning.** Flödesfyllning utifrån fungerar inte:
    flaggornas ÖVERKANT är oritad — banderollen hänger fritt från stången — så
    utsidan rinner rakt in. Priset är att en svalstjärt fylls solid: en rad genom
    klykan har bläck i båda spetsarna och fylls mellan dem. Bottenkanten är just
    flaggornas huvudsakliga särskiljare, så flaggsiffrorna är en ÖVRE gräns för
    diskriminerbarheten — den verkliga är sämre, inte bättre.
    """
    out = [0] * (w * h)
    for y in range(h):
        row = [x for x in range(w) if mask[y * w + x]]
        if row:
            for x in range(row[0], row[-1] + 1):
                out[y * w + x] = 1
    return out


def downsample(w, h, mask, size):
    """Bläckmasken → ett 1-bitars size×size-raster.

    Proportionerna behålls och resten centreras ut, annars blir en hög flagga
    kvadratisk och mäts som en annan form än den är. Box-medelvärde följt av
    50 %-tröskling = vad en formgivare gör när en form reduceras till size px,
    och resultatet är hårda pixlar utan kantutjämning (princip 10).
    """
    scale = min(size / w, size / h)
    tw, th = max(1, round(w * scale)), max(1, round(h * scale))
    ox, oy = (size - tw) // 2, (size - th) // 2
    out = [0] * (size * size)
    for ty in range(th):
        y0, y1 = int(ty * h / th), max(int(ty * h / th) + 1, int((ty + 1) * h / th))
        for tx in range(tw):
            x0, x1 = int(tx * w / tw), max(int(tx * w / tw) + 1, int((tx + 1) * w / tw))
            tot = (y1 - y0) * (x1 - x0)
            hit = sum(mask[y * w + x] for y in range(y0, y1) for x in range(x0, x1))
            if hit * 2 >= tot:
                out[(oy + ty) * size + ox + tx] = 1
    return out


def bands(counts, minrun=3):
    """Sammanhängande löpor där count > 0. Löpor kortare än `minrun` ignoreras
    (en ensam brusrad ska inte bli en egen form)."""
    out, start = [], None
    for i, c in enumerate(counts):
        if c > 0 and start is None:
            start = i
        elif c <= 0 and start is not None:
            if i - start >= minrun:
                out.append((start, i))
            start = None
    if start is not None and len(counts) - start >= minrun:
        out.append((start, len(counts)))
    return out


def grid_cells(w, h, px, threshold):
    """Segmentera ett formark i celler via bläckprojektion.

    Radband först (global radprojektion), sedan kolumnband INOM varje radband.
    En global kolumnprojektion duger inte: former på olika x i olika rader fyller
    varandras luckor och arket kollapsar till fyra kolumner i stället för tio.

    Ger [(rad, kolumn, x0, y0, x1, y1)] i läsordning — stabila id.
    """
    ink = [[1 if px[y * w + x] < threshold else 0 for x in range(w)] for y in range(h)]
    cells = []
    for row, (y0, y1) in enumerate(bands([sum(r) for r in ink])):
        colcount = [sum(ink[y][x] for y in range(y0, y1)) for x in range(w)]
        for col, (x0, x1) in enumerate(bands(colcount)):
            # Trimma cellen i höjdled till sitt eget bläck — radbandet är lika
            # högt som radens högsta form, vilket annars ger tomma marginaler
            # som förskjuter formen när den skalas ned.
            rows = [y for y in range(y0, y1) if any(ink[y][x] for x in range(x0, x1))]
            cells.append((row, col, x0, rows[0], x1, rows[-1] + 1))
    return cells
