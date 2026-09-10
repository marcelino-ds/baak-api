import io
import json
import sys

try:
    import resource
    resource.setrlimit(resource.RLIMIT_AS, (512 * 1024 * 1024,) * 2)
    resource.setrlimit(resource.RLIMIT_CPU, (15, 15))
except ImportError:
    pass

try:
    import pdfplumber
    raw = sys.stdin.buffer.read(16 * 1024 * 1024 + 1)
    if len(raw) > 16 * 1024 * 1024 or not raw.startswith(b"%PDF-"):
        raise ValueError("invalid PDF")
    with pdfplumber.open(io.BytesIO(raw)) as pdf:
        if len(pdf.pages) > 100:
            raise ValueError("PDF exceeds 100 pages")
        pages = []
        warnings = []
        length = 0
        for number, page in enumerate(pdf.pages, 1):
            text = page.extract_text(layout=False) or ""
            tables = page.extract_tables() or []
            tables = [[[cell or "" for cell in row] for row in table] for table in tables]
            length += len(text) + sum(len(c) for table in tables for row in table for c in row)
            if length > 1000000:
                raise ValueError("PDF text exceeds limit")
            if not text.strip():
                warnings.append("Page %d has no extractable text; OCR may be required." % number)
            pages.append({"number": number, "text": text, "tables": tables})
            page.close()
        result = {"title": str((pdf.metadata or {}).get("Title") or ""),
                  "page_count": len(pages), "pages": pages, "warnings": warnings}
    json.dump(result, sys.stdout, ensure_ascii=True)
except ImportError:
    sys.exit(3)
except Exception:
    sys.exit(2)
