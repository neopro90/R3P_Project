# HƯỚNG DẪN FLASH FIRMWARE R3P MINI SERVER OS (.BIN)
> Thiết bị: **Xiaomi Mi Router 3 Pro (mir3p / R3P)**  
> Vi xử lý: **MediaTek MT7621AT @ 880MHz (Dual-core 4 threads, mipsle_24kc)**  
> Bộ nhớ: **512MB DDR3 RAM + 256MB SLC NAND Flash**  
> Phiên bản nền: **OpenWrt 25.12.5 (Linux Kernel 6.12.94)**

---

## 1. Danh Sách File Firmware .bin Đính Kèm

| Tên File .bin | Dung lượng | Mục Đích Sử Dụng | Cách Flash |
|---|---|---|---|
| **`openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-factory.bin`** | ~14.88 MB | Cài mới từ Stock ROM / Breed / Telnet | Flash lần đầu từ giao diện U-Boot / Breed |
| **`openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-sysupgrade.bin`** | ~12.75 MB | Cập nhật hệ thống đang chạy OpenWrt / R3P OS | Chạy lệnh `sysupgrade -n` hoặc Web GUI |
| **`R3P-Firmware-PB-Boot.bin`** | ~11.75 MB | Dành riêng cho Bootloader PB-Boot | Vào PB-Boot Web Recovery `192.168.1.1` và flash |
| **`R3P-Recovery-Initramfs.bin`** | ~7.69 MB | Cứu hộ khẩn cấp không ghi NAND (Chạy trên RAM) | Boot qua TFTP Server hoặc PB-Boot RAM |
| **`miwifi.bin`** | ~14.88 MB | Firmware gốc Xiaomi Stock Recovery | Cứu hộ USB FAT32 (Giữ nút Reset khi cắm nguồn) |

---

## 2. Hướng Dẫn Chi Tiết Từng Phương Pháp Flash

### Cách 1: Nâng cấp trực tiếp từ OpenWrt / R3P OS (Khuyên dùng)
1. Kết nối vào router qua SSH hoặc Terminal: `ssh root@192.168.2.1` (mật khẩu mặc định: `root`).
2. Sao chép file sysupgrade vào router:
   ```bash
   scp openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-sysupgrade.bin root@192.168.2.1:/tmp/sysupgrade.bin
   ```
3. Chạy lệnh sysupgrade sạch (không giữ cấu hình cũ xung đột):
   ```bash
   sysupgrade -v -n /tmp/sysupgrade.bin
   ```
4. Đợi router tự flash vào phân vùng NAND Flash và khởi động lại trong 2-3 phút.

---

### Cách 2: Flash qua Bootloader PB-Boot (Nhanh & An Toàn Nhất)
1. Rút nguồn router Xiaomi R3P.
2. Dùng que tăm nhấn giữ nút **Reset** ở mặt sau router.
3. Cắm lại nguồn điện, tiếp tục giữ nút Reset trong khoảng **5 - 8 giây** cho đến khi đèn LED nhấp nháy màu vàng/xanh lam thì thả tay ra.
4. Đặt IP tĩnh cho máy tính của bạn:
   - IP: `192.168.1.2`
   - Subnet Mask: `255.255.255.0`
   - Gateway: `192.168.1.1`
5. Mở trình duyệt web truy cập: `http://192.168.1.1`.
6. Chọn file **`R3P-Firmware-PB-Boot.bin`** (hoặc `openwrt-...-squashfs-factory.bin`).
7. Bấm **Update / Flash** và chờ thanh tiến trình chạy 100%. Router sẽ tự khởi động lại vào R3P OS.

---

### Cách 3: Flash qua Bootloader Breed
1. Tương tự PB-Boot: Rút nguồn, nhấn giữ nút **Reset**, cắm nguồn và giữ 5 giây.
2. Máy tính cắm dây LAN vào cổng LAN 1 của router, nhận IP tự động hoặc đặt tĩnh `192.168.1.2`.
3. Mở trình duyệt web truy cập `http://192.168.1.1` vào giao diện Breed Web Recovery.
4. Chọn mục **Firmware Update (固件更新)**.
5. Tích chọn Firmware $\rightarrow$ Chọn file **`openwrt-25.12.5-ramips-mt7621-xiaomi_mi-router-3-pro-squashfs-factory.bin`**.
6. Bấm **Upload** $\rightarrow$ Xác nhận flash.

---

### Cách 4: Khôi phục Firmware Gốc Xiaomi bằng USB (Unbrick miwifi.bin)
1. Chuẩn bị 1 chiếc USB dung lượng từ 1GB - 32GB, định dạng **FAT32**.
2. Đổi tên file firmware gốc thành đúng tên: **`miwifi.bin`** và copy vào thư mục gốc của USB.
3. Cắm USB vào cổng USB 3.0 phía sau router R3P.
4. Rút nguồn router $\rightarrow$ Nhấn giữ nút **Reset** $\rightarrow$ Cắm nguồn.
5. Tiếp tục giữ nút Reset cho đến khi đèn LED nhấp nháy **màu vàng** (khoảng 10 - 12 giây) thì thả tay.
6. Router sẽ tự động đọc file `miwifi.bin` từ USB để nạp lại toàn bộ phân vùng SLC NAND. Quá trình mất từ 3 - 5 phút. Khi đèn chuyển sang **màu xanh lam (Blue)** là hoàn tất.

---

## 3. Kiểm Tra Mã Băm SHA256 (Checksum)
Trước khi flash, bạn có thể kiểm tra tính toàn vẹn của file:
```powershell
Get-FileHash -Algorithm SHA256 releases\firmware-bin\*.bin
```
So sánh với nội dung trong file `sha256sums.txt` để đảm bảo file tải về không bị lỗi hay gián đoạn.
