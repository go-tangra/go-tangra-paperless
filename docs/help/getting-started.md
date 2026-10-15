---
title: Get started
summary: Create a category, upload a document into it, watch the text extraction finish and find the document by a word inside it.
kind: guide
order: 10
tags: [paperless, getting-started, upload]
requires: documents:write
---
This walk-through files one PDF and finds it again by its content. It
assumes you are a tenant owner or administrator: they can create top-level
categories and see every document. Other users work inside the categories
shared with them; see [categories and sharing](guides/sharing.md).

## 1. Create a category

1. Open **Categories**.
2. In **New category**, enter the name `Contracts`, leave **Parent** at
   *(root)* and choose **Create**.

The category appears in the **Tree** with the path `/Contracts`. You are its
owner.

## 2. Upload a document

1. Open **Documents** and choose **Upload**.
2. Pick the **File**, for example a signed contract as PDF.
3. Optionally give it a **Name** (otherwise the file name is used), a
   **Description** and **Tags**.
4. In **Category**, choose `/Contracts`.
5. Choose **Upload**. A message confirms it, and the document appears in the
   list with status `active` and processing `pending`.

## 3. Watch the processing

Without reloading the page, the **Processing** column changes to
`processing` and then to `completed` once the text has been extracted. If it
shows `failed`, the text could not be extracted; the document is still
stored and can be downloaded.

## 4. Find it by its content

1. Open **Search**.
2. Type a word or phrase that appears inside the PDF, for example the name
   of the other party, and choose **Search**.
3. The result shows the document, its category path, a snippet with the
   matching text and a relevance rank.

## Next steps

- Edit, move, download and delete documents: [work with documents](guides/documents.md).
- Let colleagues see the category: [categories and sharing](guides/sharing.md).
- Search syntax: [search](guides/search.md).
