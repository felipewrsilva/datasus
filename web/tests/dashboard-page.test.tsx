import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { cleanup, render, screen, waitFor } from "@testing-library/react";
import DashboardPage from "@/app/page";
import type { DashboardInsights } from "@/lib/types";
import { getDashboardInsights, getFiles } from "@/lib/api";

vi.mock("@/lib/api", () => ({
  getDashboardInsights: vi.fn(),
  getFiles: vi.fn(),
}));

vi.mock("next/link", () => ({
  default: function MockLink({ children, href }: { children: ReactNode; href: string }) {
    return <a href={href}>{children}</a>;
  },
}));

vi.mock("@/components/ActionButtons", () => ({
  ScanButton: ({ onScan }: { onScan: () => void }) => (
    <button type="button" onClick={onScan}>
      Escanear
    </button>
  ),
}));

vi.mock("@/components/BrazilStateMap", () => ({
  BrazilStateMap: () => <div data-testid="brazil-map" />,
}));

const baseStats: Record<string, number> = {
  pending: 5,
  ignored: 1,
  downloading: 2,
  downloaded: 10,
  converting_csv: 1,
  csv_ready: 5,
  converting_parquet: 0,
  parquet_ready: 70,
  failed: 3,
  purged: 0,
};

function buildInsights(overrides: Partial<DashboardInsights> = {}): DashboardInsights {
  return {
    total_files: 100,
    status_counts: baseStats,
    policy_counts: { pending: 5, ignored: 1 },
    stats: baseStats,
    by_catalog: [],
    by_state: [],
    failure_reasons: [],
    pipeline_completed_count: 60,
    status_stage_mismatch_count: 0,
    by_catalog_total_mismatch: 0,
    by_state_total_mismatch: 0,
    stage_done_counts: {
      download: 80,
      csv_conversion: 70,
      parquet_conversion: 60,
    },
    ...overrides,
  };
}

describe("DashboardPage", () => {
  beforeEach(() => {
    vi.mocked(getFiles).mockResolvedValue({ items: [], total: 0 });
  });

  afterEach(() => {
    cleanup();
    vi.clearAllMocks();
  });

  it("não exibe faixas diagnósticas intrusivas quando há inconsistências", async () => {
    vi.mocked(getDashboardInsights).mockResolvedValue(
      buildInsights({
        status_stage_mismatch_count: 42,
        by_catalog_total_mismatch: 3,
        by_state_total_mismatch: -1,
      }),
    );

    render(<DashboardPage />);

    await waitFor(() => {
      expect(screen.getByRole("heading", { name: /^painel$/i })).toBeInTheDocument();
    });

    expect(screen.queryByText(/overall_status/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/file_stages/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/Divergência de totais/i)).not.toBeInTheDocument();
    expect(screen.queryByText(/status agregado/i)).not.toBeInTheDocument();
  });

  it("renderiza o resumo executivo e os cards de progresso", async () => {
    vi.mocked(getDashboardInsights).mockResolvedValue(buildInsights());

    render(<DashboardPage />);

    expect(await screen.findByRole("heading", { name: /resumo/i })).toBeInTheDocument();
    expect(await screen.findByText(/^arquivos$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^concluídos$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^download$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^csv$/i)).toBeInTheDocument();
    expect(await screen.findByText(/^parquet$/i)).toBeInTheDocument();
    expect(screen.getAllByRole("button")).toHaveLength(1);
  });
});
