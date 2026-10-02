## {#welcome} Selamat datang di Voxis Source-Available

Voxis Source-Available mengubah audio menjadi transkrip yang dapat dicari dan rangkuman AI. Anda juga dapat mengekspor hasil kerja dan menghubungkan alat lain melalui API atau MCP.

Organisasi Anda menjalankan Voxis di servernya sendiri. Untuk membuat transkrip, server mengirim audio Anda ke Speechmatics, layanan transkripsi berbasis cloud, memakai akun Speechmatics milik organisasi Anda. Audio dikirim dengan nama berkas umum, bukan nama berkas asli Anda. Setelah itu, Voxis meminta Speechmatics menghapus salinan pekerjaan tersebut.

Rangkuman dibuat oleh model AI Gemma yang dijalankan sendiri oleh organisasi Anda. Transkrip Anda hanya dikirim ke model itu.

Voxis mengenkripsi audio, transkrip, dan rangkuman yang disimpannya. Rincian berkas seperti nama dan judul, serta data akun, disimpan tanpa enkripsi tersebut. Pengelola server dapat menjangkau data Anda, jadi gunakan Voxis hanya jika Anda memercayai mereka.

## {#sign-in} Masuk dan keamanan akun

Instalasi Anda memakai realm Keycloak bawaan `voxis-oss` dan klien `voxis-oss-web`. Proses masuk membutuhkan HTTPS atau localhost karena memakai PKCE dengan S256.

- Minta administrator membuatkan akun dan memberinya akses ke Voxis.
- Jika Anda berhasil masuk tetapi melihat pesan yang meminta Anda menghubungi administrator, akun Anda belum memiliki akses. Administrator dapat memperbaikinya di Keycloak.
- Ubah kata sandi dan autentikasi multifaktor di konsol akun Keycloak.
- Voxis Source-Available tidak menyediakan pendaftaran mandiri, login sosial, verifikasi email, atau penghapusan akun. Hubungi administrator Anda.

## {#upload} Unggah audio

Pilih Unggah dari dasbor atau bilah samping. Pilih berkas audio, tunggu hingga pemindaian dan penyimpanan terenkripsi selesai, lalu mulai transkripsi dari pembaca berkas.

Di pustaka, Anda dapat mengubah rincian berkas, membuka pembaca, mencari audio dan transkrip, serta menghapus media milik Anda. Saat mencari isi transkrip, Voxis memeriksa transkrip yang sudah selesai per halaman terbatas. Gunakan **Muat lebih banyak kecocokan transkrip** saat tombol itu muncul untuk melanjutkan ke transkrip lama yang sudah selesai.

Menghapus berkas bersifat permanen. Audio, transkrip, rangkuman, serta nama, judul, dan deskripsi berkas dihapus dari server. Tindakan ini tidak dapat dibatalkan. Salinan dalam cadangan organisasi yang sudah ada tetap tersimpan sampai cadangan itu dihapus.

Kamus kustom dan paket kosakata tidak tersedia dengan Speechmatics Melia 1.

[[screenshot:dashboard-capture|Dasbor Voxis dengan pilihan unggah dan rekam]]

[[screenshot:library-search|Pencarian pustaka Voxis dengan kecocokan isi transkrip dan tombol untuk melanjutkan]]

## {#record} Rekam dan pulihkan

Pilih Rekam untuk merekam mikrofon atau audio perangkat yang didukung di peramban. Jika halaman tertutup atau jaringan terputus, perekam menawarkan pemulihan sesi yang terputus saat Anda kembali.

Sebelum terunggah, potongan rekaman Anda disimpan di peramban ini. Potongan itu dienkripsi, tetapi kuncinya disimpan di peramban yang sama, sehingga perlindungannya hanya mencegah intipan sekilas. Saat Anda keluar, Voxis menghapusnya. Jika masih ada potongan yang belum terunggah, Voxis memperingatkan Anda terlebih dahulu. Potongan itu juga dihapus saat orang lain masuk di peramban ini. Di komputer bersama, tunggu hingga unggahan selesai, lalu keluar.

Jika administrator menetapkan retensi rekaman, audio rekaman peramban dihapus permanen setelah jangka waktu itu. Transkrip dan rangkumannya tetap ada.

Rekaman standar didukung. Rekaman berprivilese tidak termasuk dalam Voxis Source-Available.

Jika transkripsi gagal, kartu aktivitasnya menampilkan kategori kegagalan yang aman. Buka item tersebut, periksa pengaturan penyedia bila perlu, lalu coba lagi.

Gambar di bawah adalah contoh terkendali dengan data sintetis. Gambar ini menunjukkan kartu kegagalan, bukan kejadian layanan yang sebenarnya.

[[screenshot:activity-failure|Kartu aktivitas Voxis terkendali yang menampilkan pesan kegagalan transkripsi yang aman]]

[[screenshot:record|Layar perekaman peramban Voxis]]

## {#summaries} Baca rangkuman dan ekspor

Pembaca menampilkan transkrip, perubahan pembicara, dan format rangkuman profesional yang tersedia. Model Gemma lokal membuat rangkuman dari kumpulan prompt publik yang dikonfigurasi. Profil rangkuman berisiko tinggi tidak aktif kecuali administrator mengaktifkannya.

Ekspor tersedia dalam PDF, DOCX, dan JSON. Ekspor PDF belum mendukung aksara Tionghoa, Jepang, atau Korea dengan andal; gunakan DOCX atau JSON untuk aksara tersebut. Ekspor BAP bersifat opsional dan tidak aktif kecuali operator mengaktifkannya.

Untuk menyimpan salinan hasil kerja Anda, unduh audionya dan ekspor setiap transkrip serta rangkuman. Anda juga dapat memakai kunci API atau alat MCP untuk mengambil banyak item sekaligus.

[[screenshot:reader|Pembaca transkrip Voxis]]

## {#admin} Administrasi, API, dan MCP

Administrator dapat memeriksa status operasional, mengatur retensi rekaman, dan mengedit prompt presentasi Gemma publik. Nama model, runtime, revisi, kuantisasi, dan data penggunaan yang tersedia muncul di area administrasi saat server melaporkannya. Kuota penyimpanan tidak diberlakukan dalam edisi ini.

Kunci API dan alat MCP mengikuti aturan organisasi dan kepemilikan yang sama dengan aplikasi. Jangan letakkan kunci dalam kode peramban, dan bagikan hanya ke alat yang Anda percayai. Kunci berhenti berfungsi sekitar satu menit setelah pemiliknya dinonaktifkan atau kehilangan akses.

[[screenshot:briefings|Tampilan rangkuman dan ekspor Voxis]]

Speechmatics juga menawarkan penerapan on-premise bagi organisasi yang ingin memproses audio di infrastruktur sendiri. Hubungi Speechmatics untuk pengaturan lisensi dan penerapan yang terpisah. Penerapan ini belum diverifikasi secara menyeluruh untuk rilis ini.
