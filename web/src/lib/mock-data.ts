import { PublicStatus, PublicDigest } from './types';

export const mockStatus: PublicStatus = {
  updated_at: new Date().toISOString(),
  device: {
    battery_level: 84,
    is_charging: true,
    ram_used_percent: 64.2,
    disk_used_percent: 42.1,
    uptime_seconds: 619200,
    uptime_hours: 172.0,
  },
  services: {
    server: 'healthy',
    gatus: 'healthy',
    tailscale: 'connected',
  },
  message: 'Pocket Sentinel Public Hub — Running in Preview Mode',
};

export const mockDigest: PublicDigest = {
  updated_at: new Date().toISOString(),
  gold: {
    world_spot: 2685.4,
    bar_buy: 43850,
    bar_sell: 43950,
    updated_at: '13:00 ICT',
  },
  stocks: [
    {
      title: 'SET Index ปิดบวก 8.4 จุด แรงหนุนกลุ่มพลังงานและอิเล็กทรอนิกส์',
      source: 'Kaohoon',
    },
    {
      title: 'ตลาดหุ้นเอเชียเปิดเขียว รับข่าวตัวเลขเงินเฟ้อสหรัฐชะลอตัว',
      source: 'Prachachat',
    },
  ],
  news: [
    {
      category: 'AI',
      summary: 'Anthropic เปิดตัวฟีเจอร์ Computer Use ให้โมเดลสั่งการหน้าจอคอมพิวเตอร์ได้อัตโนมัติ',
    },
    {
      category: 'Tech',
      summary: 'Google ประกาศอัปเกรด Gemini 2.0 พร้อมความสามารถ Multimodal ความเร็วสูง',
    },
  ],
  summary_md: `### 🤖 สรุปข่าว AI & Tech ประจำวัน
- **AI Innovations:** ตลาด AI แข่งขันเดือดด้วยโมเดลความเร็วสูงและการสั่งงาน Agent แบบ Zero-shot
- **ตลาดทองคำ:** ราคาทองสปอตโลกเคลื่อนไหวใกล้ระดับ $2,685/oz ตลาดรอความชัดเจนจากนโยบายดอกเบี้ย
- **ภาพรวมระบบ:** Pocket Sentinel ทำงานต่อเนื่อง แบตเตอรี่และระบบระบายความร้อนอยู่ในเกณฑ์สมบูรณ์`,
};

export const mockWorkspace = {
  updated_at: new Date().toISOString(),
  notes: [
    {
      id: 'nt_pub1',
      title: 'Pocket Sentinel Architecture Overview',
      content: `- [x] ARM64 pure-Go standalone binary\n- [x] Embedded SQLite with WAL mode\n- [ ] Multi-region sync to Vercel Hub\n#dev #v11 #architecture`,
      tags: ['dev', 'v11', 'architecture'],
      updated_at: new Date().toISOString(),
    },
  ],
  sheets: [
    {
      id: 'sh_pub1',
      title: 'Monthly Budget Summary',
      row_count: 5,
      col_count: 3,
      data_json: JSON.stringify({
        cells: {
          A1: { raw: 'รายการ', v: 'รายการ', style: { b: true } },
          B1: { raw: 'จำนวนเงิน', v: 'จำนวนเงิน', style: { b: true } },
          A2: { raw: 'ค่าไฟ', v: 'ค่าไฟ' },
          B2: { raw: '1450', v: 1450, fmt: 'currency' },
          A3: { raw: 'รวม', v: 'รวม', style: { b: true } },
          B3: { raw: '=SUM(B2:B2)', v: 1450, fmt: 'currency', style: { b: true } },
        },
      }),
      updated_at: new Date().toISOString(),
    },
  ],
};
