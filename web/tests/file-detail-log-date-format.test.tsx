import type { ReactNode } from "react";
import { render, screen, waitFor } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";
import FileDetailPage from "@/app/files/[id]/page";
import { getFile, getFileStages } from "@/lib/api";

vi.mock("next/link", () => ({
  default: function MockLink({ children, href }: { children: ReactNode; href: string }) {
    return <a href={href}>{children}</a>;
  },
}));

vi.mock("next/navigation", () => ({
  useParams: () => ({ id: "11111111-1111-1111-1111-111111111111" }),
}));

vi.mock("@/components/ActionButtons", () => ({
  GlobalActions: () => <div>Ações</div>,
}));

vi.mock("@/lib/api", () => ({
  getFile: vi.fn(),
  getFileStages: vi.fn(),
}));

describe("FileDetailPage log date format", () => {
  afterEach(() => {
    vi.clearAllMocks();
  });

  it("exibe logs com dd/MM/yyyy HH:mm:ss", async () => {
    vi.mocked(getFile).mockResolvedValue({
      id: "11111111-1111-1111-1111-111111111111",
      filename: "RDSP2604.dbc",
      catalog: "SIHSUS",
      state: "SP",
      year: 2026,
      month: 4,
      segment: null,
      ftp_dir: "/",
      ftp_path: "/RDSP2604.dbc",
      size_bytes: 10,
      remote_checksum: null,
      remote_timestamp: "2026-04-28T13:14:15Z",
      local_hash: null,
      root_path: "/tmp",
      dbc_path: null,
      csv_path: null,
      parquet_path: null,
      overall_status: "downloaded",
      created_at: "2026-04-28T13:14:15Z",
      updated_at: "2026-04-28T13:14:15Z",
      last_seen_at: "2026-04-28T13:14:15Z",
    });
    vi.mocked(getFileStages).mockResolvedValue({
      stages: [],
      logs: [
        {
          id: 1,
          file_id: "11111111-1111-1111-1111-111111111111",
          stage: "download",
          event_type: "completed",
          message: "ok",
          payload_json: null,
          created_at: "2026-04-28T13:14:15Z",
        },
      ],
    });

    render(<FileDetailPage />);

    await waitFor(() => {
      expect(screen.getByText("Logs de processamento")).toBeInTheDocument();
    });

    expect(screen.getByText(/\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}:\d{2}/)).toBeInTheDocument();
  });
});
