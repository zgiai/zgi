# Offline Office XML resources

The standard runtime bundles these published resources to resolve the external
schema imports used by Office document validators. The XML catalog maps exact
upstream identifiers to local files; it is not a general-purpose network proxy.
Relative schema imports remain local. Schema validation is not bypassed.

| Files | Upstream source |
| --- | --- |
| `dc.xsd`, `dcterms.xsd`, `dcmitype.xsd` | https://www.dublincore.org/schemas/xmls/qdc/2003/04/02/ |
| `xml.xsd`, `XMLSchema.dtd`, `datatypes.dtd` | https://www.w3.org/2001/03/ |

Retrieved on 2026-10-07. Upstream content is unchanged except for normalized
line endings, trailing whitespace and final newlines. Existing author notices
are retained. `SHA256SUMS` pins the distributed UTF-8 bytes, not the HTTP
response representation. Updates require reviewing provenance, hashes and the
offline verification result.

## Licenses and attribution

Portions of this software may use XML schemas Copyright © 2011 DCMI, the
Dublin Core™ Metadata Initiative. These are licensed under the Creative
Commons 4.0 Attribution license:
https://creativecommons.org/licenses/by/4.0/

See the DCMI copyright and schema reuse policy:
https://www.dublincore.org/about/copyright/

The W3C resources are redistributed under the W3C Software and Document
License. Its full notice is in `W3C-LICENSE.txt`. This software includes
material copied from XML namespace schema and XML Schema DTD resources at
https://www.w3.org/2001/03/. Copyright © 2001 World Wide Web Consortium.
Only the whitespace normalization described above has been applied.

## Runtime contract

The image sets `XML_CATALOG_FILES` to this directory's catalog. libxml2/lxml
resolves the listed schema and DTD identifiers without network access; this
does not give Python HTTP clients or arbitrary download commands access to
those sites. The directory is part of the read-only runtime Profile.

Run `python resources/verify_xml.py` from the Profile directory. It verifies
hashes, recursive schema imports, accepted and rejected XML, and rejection of
unbundled URLs. This check also runs in the Profile build.
