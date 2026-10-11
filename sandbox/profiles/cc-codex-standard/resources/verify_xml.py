"""Check bundled schema integrity and catalog resolution without network access."""

import hashlib
import os
from pathlib import Path

resource_dir = Path(__file__).resolve().parent / "xml"
for entry in (resource_dir / "SHA256SUMS").read_text().splitlines():
    digest, filename = entry.split(maxsplit=1)
    actual = hashlib.sha256((resource_dir / filename).read_bytes()).hexdigest()
    if actual != digest:
        raise RuntimeError(f"XML resource checksum mismatch: {filename}")

# libxml2 initializes its catalog on first use. Set it before importing lxml,
# in this fresh process, rather than relying on inherited build-time settings.
os.environ["XML_CATALOG_FILES"] = str(resource_dir / "catalog.xml")
from lxml import etree

parser = etree.XMLParser(no_network=True, resolve_entities=False)
for schema_name in ("dc.xsd", "dcterms.xsd", "dcmitype.xsd"):
    for host in ("dublincore.org", "www.dublincore.org"):
        for scheme in ("http", "https"):
            url = f"{scheme}://{host}/schemas/xmls/qdc/2003/04/02/{schema_name}"
            etree.XMLSchema(etree.parse(url, parser))

schema = etree.XMLSchema(etree.parse(str(resource_dir / "dc.xsd"), parser))
schema.assertValid(etree.fromstring(
    b'<dc:title xmlns:dc="http://purl.org/dc/elements/1.1/" xml:lang="en">Test</dc:title>'
))
if schema.validate(etree.fromstring(
    b'<dc:unknown xmlns:dc="http://purl.org/dc/elements/1.1/">Test</dc:unknown>'
)):
    raise RuntimeError("XML schema accepted an undeclared element")

# Do not broaden arbitrary XML resolution or turn on a generic HTTP fallback.
try:
    etree.parse("https://invalid.example/unbundled.xsd", parser)
except OSError:
    pass
else:
    raise RuntimeError("Unbundled remote schema unexpectedly resolved")
print("offline XML catalog checks passed")
