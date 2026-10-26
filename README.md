# xlsx-validator

`xlsx-validator` adalah perkakas *command-line interface* (CLI) sumber terbuka (*open-source*) berbasis Go murni (*native*) untuk memvalidasi isi buku kerja Excel (`.xlsx`) terhadap konfigurasi aturan berbasis JSON tanpa memodifikasi file aslinya.

Perkakas ini bekerja sepenuhnya secara lokal dan luring (*offline*) setelah diinstal, tanpa membutuhkan Microsoft Excel, LibreOffice, runtime Node.js, atau koneksi jaringan saat proses validasi.

---

## Daftar Isi

- [Instalasi dan Build](#instalasi-dan-build)
- [Penggunaan CLI](#penggunaan-cli)
- [Kode Keluar (Exit Codes)](#kode-keluar-exit-codes)
- [Format Konfigurasi JSON](#format-konfigurasi-json)
  - [Nilai Default](#nilai-default)
  - [Tipe Data Kolom](#tipe-data-kolom)
  - [Daftar Aturan yang Didukung](#daftar-aturan-yang-didukung)
  - [Format Layout Tanggal](#format-layout-tanggal)
- [Format Output](#format-output)
  - [Teks (Default)](#1-format-teks-default)
  - [JSON](#2-format-json)
  - [CSV](#3-format-csv)
- [Kode Error Stabil](#kode-error-stabil)
- [Contoh Penggunaan dalam Shell Script](#contoh-penggunaan-dalam-shell-script)
- [Batasan MVP & Rencana Masa Depan](#batasan-mvp--rencana-masa-depan)

---

## Instalasi dan Build

### Prasyarat

- Go versi 1.23 atau lebih baru.

### Langkah Build

Klon repositori dan bangun binary lokal menggunakan perintah:

```sh
go build ./cmd/xlsx-validator
```

Perintah di atas akan menghasilkan file *executable* bernama `xlsx-validator` (atau `xlsx-validator.exe` pada Windows) di direktori aktif.

---

## Penggunaan CLI

Bentuk perintah CLI menempatkan opsi flag sebelum argumen posisi path workbook:

```sh
# Menampilkan laporan teks ke terminal (stdout)
./xlsx-validator --config examples/employee-rules.json employees.xlsx

# Menampilkan laporan format JSON ke terminal
./xlsx-validator --config examples/employee-rules.json --format json employees.xlsx

# Menyimpan laporan JSON ke file secara aman (atomic write)
./xlsx-validator --config examples/employee-rules.json --format json --output result.json employees.xlsx

# Menyimpan laporan CSV ke file
./xlsx-validator --config examples/employee-rules.json --format csv --output result.csv employees.xlsx

# Bantuan petunjuk perintah
./xlsx-validator --help
```

### Opsi Flag

| Flag | Deskripsi | Default | Wajib? |
|------|-----------|---------|--------|
| `--config <path>` | Path ke file konfigurasi aturan JSON | - | Ya |
| `--format <tipe>` | Format laporan output: `text`, `json`, `csv` | `text` | Tidak |
| `--output <path>` | Path file tujuan untuk menulis laporan (atomic write) | stdout | Tidak |
| `--help`, `-h` | Menampilkan panduan dan keluar dengan kode 0 | - | Tidak |

> **Catatan Keamanan Penulisan Output:** Jika opsi `--output` mengarah ke file workbook input atau file konfigurasi yang sedang dibaca (termasuk melalui symlink atau hard link), eksekusi akan ditolak untuk mencegah kerusakan data asli.

---

## Kode Keluar (Exit Codes)

| Kode | Keterangan |
|------|------------|
| `0` | Validasi selesai dan **tidak ditemukan pelanggaran** (sukses). |
| `1` | Validasi selesai dengan **satu atau lebih pelanggaran** (termasuk pelanggaran struktur sheet/header). |
| `2` | Terjadi kesalahan operasional: argumen salah, konfigurasi tidak valid, workbook tidak dapat dibuka/bukan `.xlsx`, atau gagal menulis output. |

---

## Format Konfigurasi JSON

Contoh file konfigurasi `examples/employee-rules.json`:

```json
{
  "sheet": "Employees",
  "headerRow": 1,
  "skipEmptyRows": true,
  "trimSpace": true,
  "columns": {
    "EMP_NO": {
      "type": "string",
      "required": true,
      "unique": true,
      "maxLength": 20,
      "pattern": "^[0-9]+$"
    },
    "FULL_NAME": {
      "type": "string",
      "required": true,
      "minLength": 1,
      "maxLength": 150
    },
    "EMAIL": {
      "type": "string",
      "format": "email"
    },
    "JOIN_DATE": {
      "type": "date",
      "required": true,
      "dateLayouts": ["2006-01-02", "02/01/2006"],
      "allowExcelSerial": true
    },
    "SALARY": {
      "type": "number",
      "min": 0
    },
    "EMP_STATUS": {
      "type": "string",
      "allowedValues": ["PERMANENT", "CONTRACT", "PROBATION"]
    }
  }
}
```

### Nilai Default

- `headerRow`: default `1` (baris ke-1 Excel, berbasis 1-based, harus > 0).
- `skipEmptyRows`: default `true`. Nilai eksplisit `false` dihormati. Baris hanya dilewati jika seluruh sel dalam baris tersebut kosong (termasuk kolom yang tidak dikonfigurasi). Sel formula atau error Excel membuat baris dianggap tidak kosong.
- `trimSpace`: default `true`. Nilai eksplisit `false` dihormati. Pemangkasan spasi mempengaruhi pencocokan aturan; nilai asli pada laporan tetap dipertahankan.
- `type`: default `"string"` jika tidak didefinisikan.

### Tipe Data Kolom

- `string`: Teks umum (panjang dihitung berdasarkan *Unicode code points*, bukan bytes).
- `integer`: Bilangan bulat matematis. Nilai raw diverifikasi presisi tanpa konversi floating-point yang tidak akurat.
- `number`: Bilangan real berhingga (*finite*). Menolak NaN dan tak hingga (*infinity*).
- `date`: Tanggal kalender. Mendukung format teks dan nomor seri Excel.
- `boolean`: Nilai boolean *native* Excel atau string `"true"` / `"false"` (*case-insensitive*). Nilai `0`, `1`, `Y`, atau `N` bukan boolean.

### Daftar Aturan yang Didukung

| Aturan | Tipe Terkait | Deskripsi |
|--------|--------------|-----------|
| `required` | Semua | Jika `true`, sel tidak boleh kosong. Sel kosong wajib menghasilkan pelanggaran `REQUIRED`. Sel kosong opsional melewati validasi lanjutan. |
| `unique` | Semua | Menilai keunikan nilai teks normalisasi secara *case-sensitive*. Seluruh anggota kelompok duplikat akan ditandai dengan kode `DUPLICATE`. Sel kosong tidak ikut serta dalam keunikan. |
| `allowedValues` | Semua | Daftar nilai teks yang diizinkan (*case-sensitive*). Menghasilkan `NOT_ALLOWED` jika tidak cocok. |
| `minLength` | `string` | Jumlah minimum karakter (*Unicode code points*, >= 0). Menghasilkan `MIN_LENGTH`. |
| `maxLength` | `string` | Jumlah maksimum karakter (*Unicode code points*, >= 0). Menghasilkan `MAX_LENGTH`. |
| `pattern` | `string` | Regular expression Go (`regexp`). Menghasilkan `PATTERN_MISMATCH`. Gunakan `^` dan `$` untuk pencocokan penuh. |
| `format` | `string` | Format khusus. MVP mendukung `"email"` (sintaks email murni tanpa *display name*). Menghasilkan `INVALID_FORMAT`. |
| `min` | `integer`, `number` | Batas nilai minimum numerik berhingga. Menghasilkan `MIN_VALUE`. Mendukung `min: 0`. |
| `max` | `integer`, `number` | Batas nilai maksimum numerik berhingga. Menghasilkan `MAX_VALUE`. |
| `dateLayouts` | `date` | Daftar format layout parser Go (`time.Parse`). Minimal satu format jika ada input tanggal teks. |
| `allowExcelSerial` | `date` | Jika `true`, menerima nomor seri tanggal Excel (sistem 1900 atau 1904). Hari kabisat fiktif 1900-02-29 ditolak eksplisit. |

> **Validasi Konfigurasi:** Konfigurasi akan menolak *unknown fields*, duplikasi key JSON, nama kolom kosong, regex tidak valid, batas nilai tidak logis (`min > max`, `minLength > maxLength`, panjang negatif), serta kombinasi aturan yang tidak kompatibel dengan tipenya.

---

## Format Layout Tanggal

Layout tanggal pada `dateLayouts` menggunakan konvensi parser waktu Go (`time.Parse`), bukan token `YYYY-MM-DD`. Tanggal acuan referensi Go adalah `2006-01-02 15:04:05`:

- `"2006-01-02"` untuk format Tahun-Bulan-Tanggal (contoh: `2023-01-15`)
- `"02/01/2006"` untuk format Tanggal/Bulan/Tahun (contoh: `15/01/2023`)
- `"01/02/2006"` untuk format Bulan/Tanggal/Tahun (contoh: `01/15/2023`)

---

## Format Output

### 1. Format Teks (Default)

Output terminal yang mudah dibaca:

```text
File:  testdata/invalid_employees.xlsx
Sheet: Employees
Summary:
  Checked rows:           3
  Valid rows:             0
  Invalid rows:           3
  Skipped empty rows:     1
  Structure errors:       0
  Total errors:           8

Validation Status: FAILED (8 violations)
Violations:
    1. A2         DUPLICATE            duplicate value "1001" found in rows 2, 3 [EMP_NO] (value: "1001")
    2. A3         DUPLICATE            duplicate value "1001" found in rows 2, 3 [EMP_NO] (value: "1001")
    3. B3         REQUIRED             field "FULL_NAME" is required [FULL_NAME]
    4. C3         INVALID_FORMAT       value "budi@@invalid..com" is not a valid email address [EMAIL] (value: "budi@@invalid..com")
    5. D3         INVALID_TYPE         value "32/01/2023" is not a valid date [JOIN_DATE] (value: "32/01/2023")
    6. E3         MIN_VALUE            value -500000 is less than minimum 0 [SALARY] (value: "-500000")
    7. F3         NOT_ALLOWED          value "INTERN" is not in allowed values: PERMANENT, CONTRACT, PROBATION [EMP_STATUS] (value: "INTERN")
    8. A5         PATTERN_MISMATCH     value "EMP_ABC" does not match pattern "^[0-9]+$" [EMP_NO] (value: "EMP_ABC")
```

### 2. Format JSON

Struktur skema JSON terstandarisasi:

```json
{
  "file": "testdata/invalid_employees.xlsx",
  "sheet": "Employees",
  "summary": {
    "checkedRows": 3,
    "validRows": 0,
    "invalidRows": 3,
    "skippedRows": 1,
    "errorCount": 8,
    "structureErrorCount": 0
  },
  "errors": [
    {
      "sheet": "Employees",
      "row": 2,
      "cell": "A2",
      "column": "EMP_NO",
      "value": "1001",
      "code": "DUPLICATE",
      "message": "duplicate value \"1001\" found in rows 2, 3"
    }
  ]
}
```

Jika tidak ada pelanggaran, `errors` selalu bernilai array kosong `[]`.

### 3. Format CSV

File CSV berisi header dan satu record per pelanggaran:

```csv
sheet,row,cell,column,value,code,message
Employees,2,A2,EMP_NO,1001,DUPLICATE,"duplicate value ""1001"" found in rows 2, 3"
Employees,3,A3,EMP_NO,1001,DUPLICATE,"duplicate value ""1001"" found in rows 2, 3"
Employees,3,B3,FULL_NAME,,REQUIRED,"field ""FULL_NAME"" is required"
Employees,3,C3,EMAIL,budi@@invalid..com,INVALID_FORMAT,"value ""budi@@invalid..com"" is not a valid email address"
Employees,3,D3,JOIN_DATE,32/01/2023,INVALID_TYPE,"value ""32/01/2023"" is not a valid date"
Employees,3,E3,SALARY,'-500000,MIN_VALUE,value -500000 is less than minimum 0
Employees,3,F3,EMP_STATUS,INTERN,NOT_ALLOWED,"value ""INTERN"" is not in allowed values: PERMANENT, CONTRACT, PROBATION"
Employees,5,A5,EMP_NO,EMP_ABC,PATTERN_MISMATCH,"value ""EMP_ABC"" does not match pattern ""^[0-9]+$"""
```

- Jika validasi berhasil tanpa error, CSV hanya berisi baris header tanpa baris data.
- **Proteksi Injeksi Formula CSV:** Nilai teks yang diawali karakter `=`, `+`, `-`, `@`, tab (`\t`), atau carriage return (`\r`) otomatis dinetralisir dengan awalan tanda kutip tunggal `'` pada file CSV. Nilai pada JSON tetap utuh.

---

## Kode Error Stabil

| Kode | Kategori | Penjelasan |
|------|----------|------------|
| `SHEET_NOT_FOUND` | Struktur | Sheet yang dikonfigurasi tidak ditemukan di dalam workbook. |
| `MISSING_HEADER` | Struktur | Header kolom yang dikonfigurasi tidak ditemukan pada `headerRow`. |
| `DUPLICATE_HEADER` | Struktur | Terdapat dua atau lebih header non-kosong dengan nama yang sama. |
| `FORMULA_HEADER` | Struktur | Terdapat formula pada baris header. |
| `REQUIRED` | Data | Sel wajib diisi namun bernilai kosong/spasi. |
| `DUPLICATE` | Data | Nilai terdeteksi duplikat pada kolom beraturan `unique: true`. |
| `INVALID_TYPE` | Data | Tipe sel tidak cocok dengan tipe yang didefinisikan (misal: bukan integer, bukan boolean, atau tanggal tidak sah). |
| `INVALID_FORMAT` | Data | Format khusus string tidak valid (misal: format email tidak sah). |
| `MIN_LENGTH` | Data | Panjang string kurang dari `minLength`. |
| `MAX_LENGTH` | Data | Panjang string melebihi `maxLength`. |
| `MIN_VALUE` | Data | Nilai angka kurang dari batas `min`. |
| `MAX_VALUE` | Data | Nilai angka melebihi batas `max`. |
| `NOT_ALLOWED` | Data | Nilai tidak terdaftar di dalam `allowedValues`. |
| `PATTERN_MISMATCH` | Data | Nilai string tidak cocok dengan pola `pattern` regex. |
| `FORMULA_NOT_ALLOWED` | Data | Ditemukan formula pada sel data terkonfigurasi. |
| `EXCEL_ERROR` | Data | Sel berisi nilai error bawaan Excel (misal: `#VALUE!`, `#N/A`, `#REF!`). |

---

## Contoh Penggunaan dalam Shell Script

Anda dapat mengintegrasikan `xlsx-validator` dalam pipeline otomatis atau skrip bash menggunakan kode keluarnya:

```sh
#!/bin/bash
set -e

./xlsx-validator --config examples/employee-rules.json --format json --output result.json employees.xlsx
EXIT_CODE=$?

if [ $EXIT_CODE -eq 0 ]; then
    echo "Validasi berhasil: Tidak ada kesalahan."
    # Lanjutkan proses impor database
elif [ $EXIT_CODE -eq 1 ]; then
    echo "Validasi gagal: Terdapat data tidak valid. Silakan periksa result.json."
    exit 1
else
    echo "Kesalahan sistem/konfigurasi: Exit code $EXIT_CODE"
    exit 2
fi
```

---

## Batasan MVP & Rencana Masa Depan

### Fitur yang Diterapkan pada MVP

- Validasi 1 sheet per pemanggilan CLI.
- Deteksi struktur header lengkap: verifikasi keberadaan sheet, header hilang, duplikat header, dan formula header.
- Pemeriksaan tipe data: `string`, `integer`, `number`, `date`, `boolean`.
- Validasi sel formula dan sel error Excel pada baris data.
- Penanganan presisi numerik dan awalan nol pada string (misal: kode `00123`).
- Tiga format laporan: Terminal Teks, JSON, dan CSV (dengan proteksi formula injection).
- Penulisan laporan file atomik (*safe temporary rename*).
- Deteksi duplikasi key JSON pada konfigurasi.
- Urutan laporan deterministik: baris Excel, posisi kolom, lalu kode aturan.

### Ide Pengembangan Masa Depan (Di Luar MVP)

Fitur-fitur berikut sengaja ditunda dan dapat diimplementasikan di versi berikutnya:
- Konfigurasi multi-sheet dalam satu file dan satu eksekusi.
- Lookup validasi langsung ke database eksternal.
- Validasi ketergantungan antar kolom (*cross-column conditional rules*).
- Evaluasi formula dinamis (*formula calculation engine*).
- Perbaikan data otomatis (*automatic file repair/sanitization*).
- Sistem plugin dinamis.
- Antarmuka grafis (GUI) atau Web UI.
