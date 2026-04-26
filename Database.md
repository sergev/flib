# freeLib SQLite Database

freeLib uses a single SQLite database named `freeLib.sqlite` as the application catalog. The bundled empty database is stored at `src/freeLib.sqlite` and is embedded into the Qt resource file as `:/freeLib.sqlite`.

At runtime the application opens a Qt SQLite connection named `libdb`. If the configured database path does not exist, `creatDB()` copies the embedded `:/freeLib.sqlite` resource to disk. The default runtime location is `QStandardPaths::AppDataLocation/freeLib.sqlite`, unless the user setting `database_path` points to another file.

The bundled database currently contains schema version:

```sql
version = 7
version_minor = 7
```

The seed database contains no user libraries or books. It contains only two `params` rows, seven tag icons, and four default tags.

## Schema

This is the schema extracted from `src/freeLib.sqlite`.

```sql
CREATE TABLE params (
    id INTEGER PRIMARY KEY,
    name TEXT,
    value TEXT
);

CREATE TABLE icon (
    id INTEGER NOT NULL,
    dark_theme BLOB,
    light_theme BLOB,
    PRIMARY KEY(id AUTOINCREMENT)
);

CREATE TABLE tag (
    id INTEGER NOT NULL,
    name TEXT,
    id_icon INTEGER,
    PRIMARY KEY(id),
    FOREIGN KEY(id_icon) REFERENCES icon(id)
);

CREATE TABLE lib (
    id INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,
    name TEXT,
    path TEXT,
    inpx TEXT,
    version TEXT,
    firstAuthor BOOL,
    woDeleted BOOL
);

CREATE TABLE author (
    id INTEGER,
    name1 TEXT,
    name2 TEXT,
    name3 TEXT,
    id_lib INTEGER,
    FOREIGN KEY(id_lib) REFERENCES lib(id) ON DELETE CASCADE,
    PRIMARY KEY(id)
);

CREATE TABLE seria (
    id INTEGER,
    name TEXT,
    id_lib INTEGER,
    PRIMARY KEY(id),
    FOREIGN KEY(id_lib) REFERENCES lib(id) ON DELETE CASCADE
);

CREATE TABLE book (
    id INTEGER,
    name TEXT,
    star INTEGER,
    language TEXT,
    id_lib INTEGER,
    file TEXT,
    size INTEGER,
    deleted BOOL,
    date DATETIME,
    format TEXT,
    keys TEXT,
    id_inlib INTEGER,
    archive TEXT,
    first_author_id INTEGER,
    FOREIGN KEY(id_lib) REFERENCES lib(id) ON DELETE CASCADE,
    PRIMARY KEY(id)
);

CREATE TABLE book_author (
    id_book INTEGER,
    id_author INTEGER,
    id_lib INTEGER,
    FOREIGN KEY(id_lib) REFERENCES lib(id) ON DELETE CASCADE,
    FOREIGN KEY(id_author) REFERENCES author(id) ON DELETE CASCADE,
    FOREIGN KEY(id_book) REFERENCES book(id) ON DELETE CASCADE,
    PRIMARY KEY(id_book, id_author, id_lib)
);

CREATE TABLE book_genre (
    id_book INTEGER,
    id_genre INTEGER,
    id_lib INTEGER,
    PRIMARY KEY(id_book, id_genre, id_lib),
    FOREIGN KEY(id_lib) REFERENCES lib(id) ON DELETE CASCADE,
    FOREIGN KEY(id_book) REFERENCES book(id) ON DELETE CASCADE
);

CREATE TABLE book_sequence (
    id_book INTEGER REFERENCES book(id) ON DELETE CASCADE,
    id_sequence INTEGER REFERENCES seria(id) ON DELETE CASCADE,
    num_in_sequence INTEGER,
    PRIMARY KEY(id_book, id_sequence)
);

CREATE TABLE book_tag (
    id_book INTEGER NOT NULL,
    id_tag INTEGER NOT NULL,
    FOREIGN KEY(id_tag) REFERENCES tag(id) ON DELETE CASCADE,
    FOREIGN KEY(id_book) REFERENCES book(id) ON DELETE CASCADE,
    UNIQUE(id_book, id_tag) ON CONFLICT IGNORE
);

CREATE TABLE author_tag (
    id_author INTEGER NOT NULL,
    id_tag INTEGER NOT NULL,
    FOREIGN KEY(id_tag) REFERENCES tag(id) ON DELETE CASCADE,
    FOREIGN KEY(id_author) REFERENCES author(id) ON DELETE CASCADE,
    UNIQUE(id_author, id_tag) ON CONFLICT IGNORE
);

CREATE TABLE seria_tag (
    id_seria INTEGER NOT NULL,
    id_tag INTEGER NOT NULL,
    FOREIGN KEY(id_seria) REFERENCES seria(id) ON DELETE CASCADE,
    FOREIGN KEY(id_tag) REFERENCES tag(id) ON DELETE CASCADE,
    UNIQUE(id_seria, id_tag) ON CONFLICT IGNORE
);
```

SQLite also creates the internal `sqlite_sequence` table because `icon.id` and `lib.id` are `AUTOINCREMENT` columns.

## Indexes

```sql
CREATE INDEX author_sort ON author (name1 ASC, name2 ASC, name3 ASC);
CREATE INDEX seria_id ON seria (id);
CREATE INDEX seria_name ON seria (name ASC, id_lib ASC);

CREATE INDEX book_id_lib ON book (id_lib);
CREATE INDEX id_lib_archive ON book (id_lib, archive);

CREATE INDEX book_author_id_author ON book_author (id_author);
CREATE INDEX book_author_id_book ON book_author (id_book);

CREATE INDEX book_genre_id_genre ON book_genre (id_genre);
CREATE INDEX book_genre_id_book ON book_genre (id_book);

CREATE INDEX book_sequence_id_book ON book_sequence (id_book);
CREATE INDEX book_sequence_id_sequence ON book_sequence (id_sequence);

CREATE INDEX book_tag_id_tag ON book_tag (id_book);
CREATE INDEX author_tag_id_author ON author_tag (id_author);
CREATE INDEX sequence_tag_id_seria ON seria_tag (id_seria);
```

Note: `book_tag_id_tag` is a misleading index name. It indexes `book_tag(id_book)`, not `id_tag`.

There are no triggers in the bundled database.

## Entity Overview

The database has these main entity groups:

- `params`: schema version metadata.
- `lib`: configured libraries/catalogs.
- `book`: books belonging to a library.
- `author`: authors belonging to a library.
- `seria`: series belonging to a library.
- `book_author`: many-to-many book-author links.
- `book_sequence`: many-to-many book-series links plus the book number in the series.
- `book_genre`: many-to-many book-genre links.
- `icon`, `tag`: user tags and their SVG icons.
- `book_tag`, `author_tag`, `seria_tag`: many-to-many tag links for books, authors, and series.

There is no `genre` table in the current bundled schema. Genre ids in `book_genre.id_genre` are resolved against `genre.tsv`, loaded by the application into memory. Older database migrations mention a legacy `genre` table, but current databases use the TSV-based genre catalog.

## Table Details

### `params`

Stores database schema version rows.

- `id`: row id.
- `name`: parameter name.
- `value`: parameter value as text.

Current seed rows:

```text
version       7
version_minor 7
```

`openDB()` checks `version`. If it is not `7`, the application closes the database and replaces the whole file with the bundled `:/freeLib.sqlite` resource. It then checks `version_minor` and runs incremental migrations for older version-7 databases.

### `lib`

One row represents one configured book library/catalog.

- `id`: library id.
- `name`: display name.
- `path`: root directory containing book files or archives.
- `inpx`: path to the INPX source used for import/update.
- `version`: library import/version string read from the INPX data.
- `firstAuthor`: boolean import option. When true, the library is treated as using only the first author.
- `woDeleted`: boolean option meaning "without deleted"; used to hide or skip deleted records.

The application loads libraries with:

```sql
SELECT id, name, path, inpx, version, firstauthor, woDeleted
FROM lib
ORDER BY name;
```

Deleting a library enables foreign keys and deletes the `lib` row, so child rows cascade through tables that reference `lib(id)`.

### `book`

One row represents one book entry inside a library.

- `id`: book id.
- `name`: book title.
- `star`: numeric rating.
- `language`: language code as text, for example `en` or `ru`.
- `id_lib`: owning library id.
- `file`: book file path or archive member path.
- `size`: file size from the import source.
- `deleted`: soft-delete flag. A full clear can mark books deleted instead of removing them.
- `date`: import/addition date.
- `format`: book format/extension, such as `fb2`, `epub`, `mobi`.
- `keys`: keyword string imported with the book.
- `id_inlib`: book id from the source library/index.
- `archive`: archive file name if the book is stored inside an archive; otherwise empty.
- `first_author_id`: cached first author id.

Books are imported with:

```sql
INSERT INTO book(
    name, star, language, file, size, deleted, date, keys,
    id_inlib, id_lib, format, archive
) VALUES (...);
```

The application commonly treats `(id_lib, file, archive)` as the physical identity of a book during import/update duplicate checks. The `id_lib_archive` index supports archive-oriented lookups.

The `deleted` column is a soft-delete marker. `ClearLib(..., delete_only=true)` runs:

```sql
UPDATE book SET deleted = 1 WHERE id_lib = ?;
```

When `delete_only` is false, the app deletes book, author, and series rows for the library and relies on cascade cleanup.

### `author`

Stores authors for a specific library.

- `id`: author id.
- `name1`: last name / family name.
- `name2`: first name.
- `name3`: middle name / patronymic.
- `id_lib`: owning library id.

The UI builds display names from the three name fields. The import code inserts authors with:

```sql
INSERT INTO author(name1, name2, name3, id_lib)
VALUES (...);
```

`author_sort` supports sorting by the three name fields.

### `seria`

Stores book series. The project uses the spelling `seria` in table and code names.

- `id`: series id.
- `name`: series name.
- `id_lib`: owning library id.

Books are not linked to series directly from `book` in the current schema. They are linked through `book_sequence`, which allows a book to belong to more than one series.

### `book_author`

Links books to authors.

- `id_book`: references `book(id)`.
- `id_author`: references `author(id)`.
- `id_lib`: references `lib(id)`.

The primary key is `(id_book, id_author, id_lib)`, so the same author-book pair is unique within a library context. The app loads author links with:

```sql
SELECT id_book, id_author
FROM book_author
WHERE id_lib = ?;
```

The import code uses `INSERT OR IGNORE`, so duplicate links are silently skipped.

### `book_sequence`

Links books to series and stores the book number in that series.

- `id_book`: references `book(id)`.
- `id_sequence`: references `seria(id)`.
- `num_in_sequence`: number/order of the book inside the series.

The primary key is `(id_book, id_sequence)`. This supports multiple series per book.

The app loads series links with:

```sql
SELECT id_book, id_sequence, num_in_sequence
FROM book_sequence
INNER JOIN book ON book.id = id_book
WHERE book.id_lib = ?;
```

### `book_genre`

Links books to genre ids.

- `id_book`: references `book(id)`.
- `id_genre`: genre id from the external genre catalog.
- `id_lib`: references `lib(id)`.

The primary key is `(id_book, id_genre, id_lib)`.

Important: `id_genre` has no foreign key in the current schema. The app loads genre metadata from `genre.tsv` into `g::genres`. If a stored `id_genre` is unknown, `loadLibrary()` maps it to `1112`, the fallback/unsorted genre id.

During import, `cleanUnsortedGenre()` removes the fallback genre `1112` for a book when that book also has more specific genres.

### `icon`

Stores SVG icon payloads used by tags.

- `id`: icon id.
- `dark_theme`: SVG data for dark UI themes.
- `light_theme`: SVG data for light UI themes.

The seed database contains seven icons.

### `tag`

Stores user-visible tags.

- `id`: tag id.
- `name`: tag name.
- `id_icon`: optional icon id, referencing `icon(id)`.

The seed database contains these tags:

```text
1 Favorite 1
4 Reading  4
5 To read  5
6 Read     6
```

Tags can be assigned to books, authors, and series through the three tag link tables.

### `book_tag`

Links tags to books.

- `id_book`: references `book(id)`.
- `id_tag`: references `tag(id)`.

The pair `(id_book, id_tag)` is unique with `ON CONFLICT IGNORE`.

### `author_tag`

Links tags to authors.

- `id_author`: references `author(id)`.
- `id_tag`: references `tag(id)`.

The pair `(id_author, id_tag)` is unique with `ON CONFLICT IGNORE`.

### `seria_tag`

Links tags to series.

- `id_seria`: references `seria(id)`.
- `id_tag`: references `tag(id)`.

The pair `(id_seria, id_tag)` is unique with `ON CONFLICT IGNORE`.

## Relationships

The core ownership graph is:

```text
lib
  -> book
  -> author
  -> seria

book <-> author through book_author
book <-> seria  through book_sequence
book <-> genre  through book_genre and external genre.tsv

tag <-> book   through book_tag
tag <-> author through author_tag
tag <-> seria  through seria_tag

icon -> tag through tag.id_icon
```

Most child relationships use `ON DELETE CASCADE`. The application explicitly enables foreign keys with `PRAGMA foreign_keys = ON` before destructive operations that rely on cascading.

## Genre Data

Genres are special because the current SQLite schema stores only links from books to numeric genre ids. The genre catalog itself is loaded from `genre.tsv`.

`loadGenres()` first looks for a user-writable `genre.tsv` in the application data directory. If it does not exist, it falls back to the Qt resource `:/genre.tsv`.

Each loaded genre entry is kept in memory as `g::genres`. The app uses that in-memory map for display names, hierarchy, OPDS output, filtering, and import key mapping. `book_genre.id_genre` therefore stores ids into this external catalog, not rows in a SQL table.

## Annotations and Covers

Book annotations and covers are not stored in `freeLib.sqlite`. They are extracted from book files on demand through `BookFile` and format readers. Some code caches annotation text in memory on `SBook`, but there is no annotation table.

## Archive Handling

The `book.archive` column stores the archive file name when a book is inside an archive. The `book.file` column then stores the member path/name inside that archive. For non-archived books, `archive` is empty and `file` is the relative book file path under `lib.path`.

Import and duplicate detection use `file` and `archive` together, scoped by `id_lib`.

## Versioning and Migrations

Database versioning is stored in `params`, not in `PRAGMA user_version`.

`openDB()` performs this flow:

1. Open the configured database path as connection `libdb`.
2. Read `params.name = 'version'`.
3. If `version != 7`, replace the whole database file from `:/freeLib.sqlite`.
4. Read `params.name = 'version_minor'`.
5. Apply migrations for old minor versions.
6. Enable foreign keys with `PRAGMA foreign_keys = 1`.

The current migration code handles old version-7 databases:

- `version_minor == 0`: renames legacy `favorite` to `tag`, renames `janre` to `genre`, creates `icon`, recreates `tag`, seeds tag icons and default tags, and creates tag link tables.
- `version_minor < 4`: rebuilds `book_tag`, `author_tag`, and `seria_tag` with current unique constraints and cascading foreign keys.
- `version_minor < 5`: rebuilds `book_genre` with `(id_book, id_genre, id_lib)` primary key, adds `book_genre` indexes, and updates legacy genre keys.
- `version_minor < 3`: rebuilds `lib` with the `version` column.
- `version_minor < 2`: rebuilds `author`, `book`, `book_author`, and `seria`; adds author, book-author, and series indexes.
- `version_minor < 6`: creates `book_sequence`, migrates old `book.id_seria` and `book.num_in_seria` into it, rebuilds `book` without direct series columns, adds book indexes, sets `version_minor` to 6, and runs `VACUUM`.
- `version_minor < 7`: remaps legacy genre id `131` to `105`, updates legacy genre keys, deletes the old genre row, and sets `version_minor` to 7.

Some migration steps reference tables that do not exist in the current bundled schema, such as the legacy `genre` table. Those references are only for older user databases.

## Seed Data

The bundled `src/freeLib.sqlite` has these row counts:

```text
params        2
icon          7
tag           4
lib           0
book          0
author        0
seria         0
book_author   0
book_genre    0
book_sequence 0
book_tag      0
author_tag    0
seria_tag     0
```

The application populates user data by adding libraries and importing book metadata from library/index files.
