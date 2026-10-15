---
title: "Example: a searchable contract archive"
summary: File signed contracts by year, describe them so they are easy to find, and pull out the right one with phrase and exclusion searches.
kind: example
order: 50
tags: [paperless, example, search, categories, contracts]
requires: search:read
---
**Goal.** Legal keeps every signed contract as a PDF. When someone asks
"which supplier contracts mention a 30-day notice period?", you want the
answer from **Search** instead of opening files one by one.

This example is done by a tenant administrator, who can create top-level
categories and see every document.

## 1. File by year

In **Categories**, create `Contracts` at the root, then select it and use
**Add subcategory** to create `2025` and `2026`. The tree shows
`/Contracts/2025` and `/Contracts/2026`.

## 2. Upload with a useful name and description

For each contract, choose **Upload** in **Documents** and fill in:

| Field | Example |
|---|---|
| **File** | `acme-supply-signed.pdf` |
| **Name** | `ACME supply contract` |
| **Description** | `Supply of spare parts, signed March 2026, renews yearly` |
| **Category** | `/Contracts/2026` |
| **Tags** | `counterparty` = `ACME`, `type` = `supply` |

The name and description are searchable as soon as the document is saved.
The text inside the PDF becomes searchable when **Processing** shows
`completed`. Tags are shown on the document but are not part of the search.

## 3. Ask the questions

| You want | Type in Search |
|---|---|
| contracts that mention a notice period of 30 days | `"30 days notice"` |
| supply contracts, but not drafts | `supply contract -draft` |
| anything about renewal or extension | `renewal or extension` |
| the ACME contract | `acme` |

Each hit shows the document name, its category path (so you can tell 2025
from 2026), a snippet of the matching text and a rank: higher means a
better match.
