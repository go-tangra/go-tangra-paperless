---
title: Paperless
summary: Store your tenant's documents, file them in categories, share them with people and roles, and find them by the text inside.
kind: guide
order: 0
tags: [paperless, documents, overview]
---
Paperless is your tenant's document store. You upload files, file them in a
tree of **categories**, and decide who may read, change or share them. After
upload, paperless extracts the text of each document in the background, so
you can search for words that appear inside PDFs and office files, not only
in their names.

Other modules keep their files here too. The Assets module, for example,
stores the documents attached to assets, consumables and licenses under the
category `Assets`.

## Concepts

- **Document**: one file with a name, an optional description, a category and
  tags. Paperless keeps a SHA-256 checksum of the bytes.
- **Category**: a folder in a tree, for example `/Finance/2026`. A document
  belongs to at most one category.
- **Processing**: after upload, the text is extracted in the background. The
  **Processing** column shows `pending`, `processing`, `completed` or
  `failed`, and updates live.
- **Access**: each document and category has its own grants: `viewer`,
  `sharer`, `editor` or `owner`, given to a user, a role or everyone in the
  tenant. A grant on a category applies to everything inside it. Whoever
  uploads a document or creates a category becomes its owner.

Tenant owners and administrators can see and manage every document and
category, whatever the grants say.

## Two layers of permission

1. Your **role** gives you module permissions such as `documents:read` or
   `documents:write`. They decide which pages and actions you can use at all.
2. The **grants** on a document or category decide which documents and
   categories you can use them on.

You need both. For example, the *Paperless editor* role lets you change
documents, but only those on which you are `editor` or `owner`.

## Pages in the portal

| Page | Shows | Needs |
|---|---|---|
| Documents | the document list; upload, edit, download, share, delete | `documents:read` |
| Categories | the category tree; create, move, share, delete | `categories:read` |
| Search | full-text search | `search:read` |
| Dashboard | counts, storage used, processing backlog | `stats:read` |

## Where to go next

- [Get started](getting-started.md): upload your first document and find it again.
- [Work with documents](guides/documents.md)
- [Categories and sharing](guides/sharing.md)
- [Search](guides/search.md)
- [Example: a searchable contract archive](examples/contract-archive.md)
- [Reference](reference.md): fields, states, permissions, roles, API and limits.
