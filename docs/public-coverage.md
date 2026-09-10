# Public BAAK Coverage

This document describes the public BAAK data exposed by `baak-api`. The API is
read-only and retrieves public BAAK content when an endpoint is requested or its
cache needs refreshing.

## Available data

| BAAK data | API endpoint |
| --- | --- |
| Homepage summary, announcements, calendar, and links | `/public/ringkasan` |
| Class schedules | `/jadwal/{kelas}`, `/jadwal?q=...` |
| Main examination schedule | `/ujian-utama?jurusan=...` |
| Final examination schedule | `/uas/{kelas}` |
| Academic calendar | `/kalender`, `/public/kalender` |
| Course catalogue | `/mata-kuliah`, `/public/dokumen-mata-kuliah` |
| Class advisors | `/dosen-wali/{tingkat}` |
| Course coordinators | `/koordinator` |
| Scientific writing advisors | `/pembimbing-pi` |
| Class and examination guides | `/panduan-kuliah`, `/jadwal-ujian`, `/ujian-bentrok` |
| Study plan form | `/frs`, `/public/dokumen-frs` |
| Public administration procedures | `/layanan/{slug}` |
| Guidance books | `/buku-pedoman`, `/public/dokumen-buku-pedoman` |
| Calendar archive | `/arsip-kalender`, `/public/dokumen-kalender` |
| News and news details | `/berita`, `/berita/{id}` |
| New class search | `/kelasbaru/{query}`, `/cariKelasBaru?...` |
| New student and KTM information | `/mahasiswabaru/{query}`, `/cariMhsBaru?...` |
| RPS, thesis, and department websites | `/situs` |
| Service desk hours | `/loket` |

Table data supports pagination or search when the BAAK source provides it.
News details include an ID, title, publication date, author, content, and
attachment links. External websites remain links; this API does not proxy their
pages or actions.

## PDF documents

Text and table extraction is available for PDFs listed in four official catalogues:

- `mata-kuliah`
- `buku-pedoman`
- `frs`
- `kalender`

Read `data.links[].id` from a catalogue and call
`/dokumen/{kategori}/{id}/teks`. The endpoint rejects free-form URLs and only
downloads official paths found in the catalogue.

PDF attachments from news, administration forms, and other public pages may still
appear as links in page responses, but are not part of the text-extraction
catalogues yet. `.doc` and `.docx` files are not handled by the PDF extractor.

Extraction returns general text and tables. It does not convert documents into
semantic academic fields such as course codes, credits, or semesters. Scanned
PDFs may return a warning because OCR is not enabled.

## Freshness and Cloudflare

Public page metadata includes `fetched_at`, `expires_at`, and `content_hash`.
Default TTLs are 5 minutes for news, 1 hour for document catalogues, 24 hours for
PDF results, and 10 minutes for other public sections. Legacy schedule endpoints
keep their own TTL configuration.

The cache refreshes when a request arrives after the TTL expires. There is no
automatic polling or webhook. If BAAK is unavailable, an upstream error does not
delete a still-valid cached response.

BAAK uses Cloudflare. FlareSolverr is an HTML fallback; it does not provide binary
PDF downloads and does not guarantee that every source request will succeed.

## Optional OCR

[Baidu Unlimited-OCR](https://github.com/baidu/Unlimited-OCR) supports multi-page
PDF input and markdown output. Its official recipe requires a roughly 3B-parameter
model, at least 8 GB of VRAM, a dedicated vLLM image, and custom inference settings.
If enabled, OCR should run as a separate GPU worker with a queue, page limits,
timeouts, and a `pdfplumber` fallback. OCR output still needs a parser before it
can be treated as structured academic data.
