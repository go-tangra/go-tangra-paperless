---
title: Categories and sharing
summary: Build the category tree, and grant users, roles or the whole tenant access to documents and categories, with optional expiry.
kind: guide
order: 30
tags: [paperless, categories, sharing, permissions, access]
requires: categories:read
---
## The category tree

Open **Categories**. The **Tree** shows every category; choose one to see its
path and depth and the actions for it.

| Action | Where | Needs |
|---|---|---|
| Create a top-level category | **New category**, **Parent** *(root)*, **Create** | `categories:manage`; tenant owner or administrator |
| Create a subcategory | select the parent, **Add subcategory**, or pick a **Parent** in **New category** | `categories:manage`; `editor` or `owner` on the parent |
| Move | select it, choose **Move to parent**, **Move** | `categories:manage`; `editor` or `owner` on the category |
| Delete | select it, **Delete** | `categories:manage`; `editor` or `owner` on the category |

- In the portal, names may not contain `/` and are at most 120 characters. A name must be
  unique under its parent, and top-level names are unique in the tenant.
- A category is shown by its path, for example `/Finance/2026`. Moving a
  category moves its whole subtree; you cannot move a category under itself
  or one of its subcategories.
- A category with subcategories cannot be deleted. Deleting a category does
  not delete the documents in it: they stay, without a category. Move them
  first if you want to keep them filed.

> Moving or renaming a category does not yet update the path stored on the
> documents inside it, so access granted on the new parent does not reach
> them. After moving a category, open each document and save it with its
> category again.

## Access levels

| Level | Read and download | Change and delete | Share |
|---|:-:|:-:|:-:|
| `viewer` | yes | | |
| `sharer` | yes | | yes |
| `editor` | yes | yes | |
| `owner` | yes | yes | yes |

A grant names a **user**, a **role** (everyone holding it) or **everyone in
the tenant**, and may expire on a date. Access is the union of every grant
that reaches you:

- grants on the document or category itself;
- grants on any category above it in the tree.

A grant to everyone in the tenant works like any other grant: it covers the
item you share it on and, for a category, everything inside it.

Tenant owners and administrators always have `owner` access.

## Share a document or category

You need `permissions:manage` and `owner` or `sharer` access to the item.

1. Open the document (in **Documents**) or select the category (in
   **Categories**) and choose **Share access**.
2. The drawer lists the current grants and your own level.
3. In **User or role**, pick *Everyone in the tenant*, a role, or type to
   find a user.
4. Choose the **Permission**. You can grant only up to your own level.
5. Optionally set **Expires**.
6. Choose **Grant**.

To remove a grant, choose the cross next to it.

- A `sharer` can grant and revoke only `viewer` and `sharer`. Granting or
  revoking `editor` or `owner` needs `owner` access, so a sharer cannot
  promote anyone or remove an owner.
- A grant on a category applies to every document and subcategory inside it.
