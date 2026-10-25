//go:build aieval

package aiassist

// evalCase is one request the baseline measurement (plan 2026-10-25, C1) puts to the real model.
//
// Group says what the case is for:
//   - "control": everything asked for is within what the assistant can write today, so ANY capability gap the
//     model records is a false one;
//   - "gap": the request needs something the assistant cannot write; the model must record it. A gap
//     satisfies the case when its key is in WantKeys (a need the register already knows) or its class is in
//     WantClasses (a need the register does not know yet, where only the family can be asserted);
//   - "template": a ready-made application covers it; the model must point at WantTemplate (or record a gap);
//   - "table": the request names columns, a search and a status filter, which the assistant can now write (D2,
//     plan 2026-10-25): the case is satisfied by a change that carries a menu entry with a table (list.columns)
//     and no capability gap.
//
// The expectations are written by hand from the case corpus and the register, not read from the log: the log
// held six rows when this was written. They are deliberately coarse -- a key is a slug the model chooses, so a
// new need can only be asserted by its class -- and the harness reports the keys it actually got.
type evalCase struct {
	ID           string
	Group        string
	Prompt       string
	WantKeys     []string
	WantClasses  []string
	WantTemplate string
}

var evalCases = []evalCase{
	// ---- control: nothing here needs a capability the assistant lacks
	{ID: "helpdesk", Group: "control", Prompt: "Buatkan aplikasi helpdesk internal: karyawan membuat tiket dengan judul, deskripsi dan prioritas (rendah, sedang, tinggi); petugas IT mengambil tiket lalu statusnya baru, dikerjakan, selesai. Petugas diberi tahu lewat email saat ada tiket baru."},
	{ID: "leave-request", Group: "control", Prompt: "Saya butuh aplikasi pengajuan cuti. Karyawan mengisi tanggal mulai, tanggal selesai dan alasan. Atasan menyetujui atau menolak. Statusnya diajukan, disetujui, ditolak."},
	{ID: "petty-cash", Group: "control", Prompt: "Buat aplikasi kas kecil: setiap pengeluaran punya tanggal, keterangan, jumlah uang, dan kategori. Bendahara harus menyetujui sebelum dianggap tercatat. Statusnya diajukan, disetujui, ditolak."},
	{ID: "maintenance", Group: "control", Prompt: "Aplikasi permintaan perawatan gedung: pelapor mengisi lokasi, masalah dan tanggal target; teknisi mengerjakan. Tampilkan sebagai papan per status (baru, dikerjakan, selesai) dan sebagai tabel."},
	{ID: "inventory", Group: "control", Prompt: "Aplikasi stok gudang sederhana: barang punya kode, nama, jumlah, dan status (tersedia, menipis, habis). Gudang mengubah status secara manual."},
	{ID: "lending", Group: "control", Prompt: "Aplikasi peminjaman alat: peminjam, alat yang dipinjam, tanggal pinjam, tanggal jatuh tempo. Status dipinjam, dikembalikan. Tampilkan daftar pinjaman yang jatuh tempo."},

	{ID: "visitor-log", Group: "control", Prompt: "Buku tamu kantor digital: nama tamu, perusahaan, orang yang dituju (salah satu karyawan), keperluan, jam datang. Status menunggu, diterima, selesai."},
	{ID: "asset-register", Group: "control", Prompt: "Daftar aset kantor: kode aset, nama, kategori (laptop, monitor, furnitur), penanggung jawab (karyawan), tanggal beli, nilai perolehan. Status dipakai, diperbaiki, dihapuskan."},
	{ID: "feedback", Group: "control", Prompt: "Tempat menampung masukan pelanggan: judul, isi masukan, jenis (saran, keluhan, pujian), pengirim. Tim produk menandainya baru, dibahas, ditindaklanjuti, ditutup."},
	{ID: "training", Group: "control", Prompt: "Pendaftaran pelatihan karyawan: nama pelatihan, peserta (karyawan), tanggal pelaksanaan, biaya. Atasan menyetujui atau menolak pendaftaran. Status diajukan, disetujui, ditolak, selesai."},

	{ID: "contacts-table", Group: "table", Prompt: "Aplikasi daftar kontak pelanggan: nama, perusahaan, email, status (prospek, aktif, nonaktif). Saya mau melihatnya sebagai tabel dengan kolom nama, perusahaan dan status, yang bisa dicari berdasarkan nama atau perusahaan dan difilter berdasarkan status."},
	{ID: "orders-table", Group: "table", Prompt: "Aplikasi pesanan: nomor pesanan, pelanggan, tanggal, nilai (uang), status (baru, diproses, selesai). Tampilkan semua pesanan sebagai tabel dengan kolom nomor, pelanggan, nilai dan status; ada pencarian nomor atau pelanggan dan filter status."},

	// ---- gap: a need the assistant cannot write, each recorded as the capability the runtime should have
	{ID: "pm-full", Group: "gap", Prompt: "Buatkan aplikasi manajemen proyek untuk tim kreatif: proyek, daftar (list), dan tugas. Tugas ditampilkan sebagai papan per daftar dengan drag untuk memindahkan, juga sebagai kalender tanggal jatuh tempo, dan punya halaman detail dengan tombol aksi. Admin proyek bisa menambah status dan field sendiri lewat menu pengaturan.", WantKeys: []string{"user-managed-fields-and-statuses"}, WantClasses: []string{ClassUIComponent, ClassConfiguration}},
	{ID: "settings-statuses", Group: "gap", Prompt: "Buat aplikasi pelacak bug. Admin aplikasi harus bisa menambah, mengganti nama dan mengurutkan status sendiri dari layar pengaturan, tanpa meminta developer.", WantKeys: []string{"user-managed-fields-and-statuses"}, WantClasses: []string{ClassConfiguration}},
	{ID: "card-in-column", Group: "gap", Prompt: "Buat papan tugas dengan kolom per daftar. Saat saya menambah kartu langsung dari sebuah kolom, daftar kartu itu otomatis terisi sesuai kolomnya.", WantKeys: []string{"create-from-grouping-sets-the-group"}, WantClasses: []string{ClassWorkflowLogic}},
	{ID: "explain-blocked", Group: "gap", Prompt: "Buat aplikasi permintaan pembelian. Kalau seseorang tidak bisa membuat permintaan, layar harus menjelaskan peran apa yang dibutuhkan dan siapa yang bisa memberikannya.", WantKeys: []string{"explain-why-an-action-is-unavailable"}, WantClasses: []string{ClassWorkflowLogic}},
	{ID: "calendar", Group: "gap", Prompt: "Aplikasi agenda kegiatan sekolah: kegiatan punya tanggal dan penanggung jawab. Saya mau melihatnya sebagai kalender bulanan, bukan daftar.", WantClasses: []string{ClassUIComponent}},
	{ID: "dashboard-chart", Group: "gap", Prompt: "Aplikasi penjualan: pesanan dengan tanggal, pelanggan dan nilai. Saya mau dasbor dengan grafik total penjualan per bulan dan per pelanggan.", WantClasses: []string{ClassUIComponent, ClassDataModel}},
	{ID: "voting-approval", Group: "gap", Prompt: "Aplikasi persetujuan anggaran: sebuah permintaan harus disetujui oleh minimal 2 dari 3 anggota komite, bukan satu orang.", WantClasses: []string{ClassWorkflowLogic}},
	{ID: "multilevel-approval", Group: "template", Prompt: "Saya butuh alur persetujuan dokumen bertingkat: staf mengajukan dokumen, lalu manajer, lalu direktur, lengkap dengan tanda tangan dan lampiran.", WantTemplate: "Document Approval"},
	{ID: "csv-import", Group: "gap", Prompt: "Aplikasi daftar kontak pelanggan. Saya sudah punya 2000 kontak di file Excel; tolong ada fitur impor dari file itu.", WantClasses: []string{ClassIntegration}},
	{ID: "pdf-invoice", Group: "gap", Prompt: "Aplikasi faktur: setiap faktur punya pelanggan, tanggal dan total. Pengguna harus bisa mengunduh faktur sebagai PDF yang rapi untuk dikirim ke pelanggan.", WantClasses: []string{ClassUIComponent}},
	{ID: "auto-number", Group: "gap", Prompt: "Aplikasi surat keluar: setiap surat otomatis mendapat nomor urut seperti SK-2026-0001 yang tidak bisa diubah dan tidak dipakai ulang.", WantClasses: []string{ClassDataModel}},
	{ID: "whatsapp", Group: "gap", Prompt: "Aplikasi antrean pasien klinik. Saat giliran pasien hampir tiba, kirim pesan WhatsApp ke nomornya.", WantClasses: []string{ClassIntegration}},
	{ID: "progress-rollup", Group: "gap", Prompt: "Aplikasi proyek dan tugas: setiap proyek menampilkan persentase tugasnya yang sudah selesai, dihitung otomatis dari tugas-tugasnya, dan diurutkan dari yang paling tertinggal.", WantClasses: []string{ClassDataModel}},
	{ID: "own-records-only", Group: "gap", Prompt: "Aplikasi laporan kunjungan sales: setiap sales hanya boleh melihat laporan miliknya sendiri, dan manajer wilayah melihat semua laporan di wilayahnya.", WantClasses: []string{ClassWorkflowLogic}},
}
