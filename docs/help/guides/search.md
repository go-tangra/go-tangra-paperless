---
title: Search
summary: Find documents by words in their name, description or extracted text, with quoted phrases, OR and excluded words.
kind: guide
order: 40
tags: [paperless, search, full-text]
requires: search:read
---
Open **Search**, type at least two characters and choose **Search** (or press
Enter). Paperless searches the name, the description and the text extracted
from each document, and lists up to 25 matches, best first.

Each hit shows the document name, its category path, a snippet of the
matching text, the file type and a relevance **rank**.

## What you can type

The query uses web-search style syntax (English word stemming, so `invoice`
also finds `invoices`):

| Query | Finds documents with |
|---|---|
| `supply contract` | both words, anywhere |
| `"supply contract"` | the exact phrase |
| `invoice or receipt` | either word |
| `contract -draft` | `contract` but not `draft` |

Very common words (`the`, `and`) are ignored.

## What is searched

- Only documents you may read appear: tenant owners and administrators, and
  users covered by a grant to everyone in the tenant, search everything;
  other users search the documents shared with them directly.
- Deleted documents are never found.
- A document is searchable by its text only after its processing is
  `completed`. Before that, and when extraction failed, only its name and
  description are searched.
