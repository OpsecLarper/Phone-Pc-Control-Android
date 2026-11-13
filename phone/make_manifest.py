"""Writes a binary (compiled) AndroidManifest.xml without needing aapt2."""
import struct, sys

ANDROID_NS = "http://schemas.android.com/apk/res/android"
ATTR_IDS = {
    "theme": 0x01010000, "label": 0x01010001, "name": 0x01010003, "exported": 0x01010010,
    "configChanges": 0x0101001F, "minSdkVersion": 0x0101020C, "versionCode": 0x0101021B,
    "versionName": 0x0101021C, "windowSoftInputMode": 0x0101022B, "targetSdkVersion": 0x01010270,
    "hardwareAccelerated": 0x010102D3, "usesCleartextTraffic": 0x010104EC,
}
T_REF, T_STR, T_DEC, T_HEX, T_BOOL = 0x01, 0x03, 0x10, 0x11, 0x12

# (tag, [(android-ns?, attr, type, value)], children)
TREE = ("manifest", [(False, "package", T_STR, "com.phoneremote.app"),
                     (True, "versionCode", T_DEC, 1),
                     (True, "versionName", T_STR, "1.0")], [
    ("uses-sdk", [(True, "minSdkVersion", T_DEC, 24), (True, "targetSdkVersion", T_DEC, 34)], []),
    ("uses-permission", [(True, "name", T_STR, "android.permission.INTERNET")], []),
    ("application", [(True, "label", T_STR, "Phone Remote"),
                     (True, "theme", T_REF, 0x01030129),  # Theme.DeviceDefault.NoActionBar
                     (True, "hardwareAccelerated", T_BOOL, True),
                     (True, "usesCleartextTraffic", T_BOOL, True)], [
        ("activity", [(True, "name", T_STR, "com.phoneremote.app.MainActivity"),
                      (True, "label", T_STR, "Phone Remote"),
                      (True, "exported", T_BOOL, True),
                      (True, "configChanges", T_HEX, 0x80 | 0x20 | 0x400 | 0x100 | 0x800 | 0x10),
                      (True, "windowSoftInputMode", T_HEX, 0x10)], [
            ("intent-filter", [], [
                ("action", [(True, "name", T_STR, "android.intent.action.MAIN")], []),
                ("category", [(True, "name", T_STR, "android.intent.category.LAUNCHER")], []),
            ]),
        ]),
    ]),
])

# ---- string pool: resource-id'd attribute names first, in resource-map order ----
used_attrs = []
def walk_attrs(node):
    for ns, a, *_ in node[1]:
        if ns and a not in used_attrs: used_attrs.append(a)
    for c in node[2]: walk_attrs(c)
walk_attrs(TREE)
used_attrs.sort(key=lambda a: ATTR_IDS[a])
strings = list(used_attrs)
def S(s):
    if s not in strings: strings.append(s)
    return strings.index(s)
def collect(node):
    S(node[0])
    for ns, a, t, v in node[1]:
        S(a)
        if t == T_STR: S(v)
    for c in node[2]: collect(c)
S("android"); S(ANDROID_NS); collect(TREE)

def chunk(typ, header, body):
    hsize = 8 + len(header)
    return struct.pack("<HHI", typ, hsize, hsize + len(body)) + header + body

def string_pool():
    offsets, data = [], b""
    for s in strings:
        offsets.append(len(data))
        u = s.encode("utf-16-le")
        data += struct.pack("<H", len(s)) + u + b"\0\0"
    while len(data) % 4: data += b"\0"
    starts = 28 + 4 * len(strings)
    header = struct.pack("<IIIII", len(strings), 0, 0, starts, 0)
    return chunk(0x0001, header, b"".join(struct.pack("<I", o) for o in offsets) + data)

def res_map():
    return chunk(0x0180, b"", b"".join(struct.pack("<I", ATTR_IDS[a]) for a in used_attrs))

NONE = 0xFFFFFFFF
line = [1]
def node_bytes(node):
    tag, attrs, kids = node
    attrs = sorted(attrs, key=lambda x: ATTR_IDS.get(x[1], 0) if x[0] else 0)
    ab = b""
    for ns, a, t, v in attrs:
        nsi = S(ANDROID_NS) if ns else NONE
        if t == T_STR: raw, data = S(v), S(v)
        elif t == T_BOOL: raw, data = NONE, 0xFFFFFFFF if v else 0
        else: raw, data = NONE, v
        ab += struct.pack("<IIIHBBI", nsi, S(a), raw, 8, 0, t, data)
    ln = line[0]; line[0] += 1
    start = chunk(0x0102, struct.pack("<II", ln, NONE),
                  struct.pack("<IIHHHHHH", NONE, S(tag), 20, 20, len(attrs), 0, 0, 0) + ab)
    body = b"".join(node_bytes(k) for k in kids)
    end = chunk(0x0103, struct.pack("<II", line[0], NONE), struct.pack("<II", NONE, S(tag)))
    line[0] += 1
    return start + body + end

ns_start = chunk(0x0100, struct.pack("<II", 1, NONE), struct.pack("<II", S("android"), S(ANDROID_NS)))
tree = node_bytes(TREE)
ns_end = chunk(0x0101, struct.pack("<II", line[0], NONE), struct.pack("<II", S("android"), S(ANDROID_NS)))
body = string_pool() + res_map() + ns_start + tree + ns_end
out = chunk(0x0003, b"", body)
open(sys.argv[1], "wb").write(out)
print("wrote", sys.argv[1], len(out), "bytes")
