---
title: Work with documents
summary: Upload, edit, move, download and delete documents, and follow their text extraction.
kind: guide
order: 20
tags: [paperless, documents, upload, processing, delete]
requires: documents:read
---
Open **Documents** to see the list, newest first. Choose a column header to
sort the whole list. **Status** and **Processing** narrow it down.

The list shows every document of the tenant, so it opens only for tenant
owners and administrators. Other users find the documents shared with them
through [search](search.md).

## Upload

You need `documents:write` and, when you choose a category, `editor` or
`owner` access to it.

1. Choose **Upload**.
2. Pick the **File**. The form refuses files over 100 MiB; your operator may
   have set a lower limit.
3. Optionally fill in **Name** (default: the file name), **Description**,
   **Category** and **Tags** (up to 32 key/value pairs).
4. Choose **Upload**.

You become the document's `owner`. Your operator may restrict which file
types are accepted; a refused type answers *unsupported media type*.

## Text extraction

Every new document is queued for text extraction. The **Processing** column
updates live:

| Processing | Meaning |
|---|---|
| `pending` | waiting for a worker, or waiting to retry after an error |
| `processing` | the text is being extracted |
| `completed` | done; the text is searchable |
| `failed` | extraction failed after the retries; the document is still stored |

A failed extraction is retried, with a growing pause, up to three times.
A document whose extraction failed can still be downloaded and found by its
name and description.

## Open, edit and move

Choose a document to open it. The drawer shows its status, source and
processing state, the file name, size, type, checksum, category path,
creator and creation time.

- Change the **Name**, **Description**, **Category** or **Tags** and choose
  **Save**. This needs `documents:write` and `editor` or `owner` access to the
  document.
- **Download** opens the file in a new tab.

Clearing the **Category** field does not take a document out of its
category; choose another category instead.

## Delete

**Delete** in the drawer needs `documents:delete` and `editor` or `owner`
access. The document is moved to the `deleted` state: it disappears from the
list and from search, but is kept. To see deleted documents, set **Status**
to `deleted`. The portal cannot restore a deleted document.

The API can also delete for good (`hard=true`), which removes the file too.

## Share

**Share access** (with `permissions:manage`) opens the access drawer for the
document. See [categories and sharing](sharing.md).
