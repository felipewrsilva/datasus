import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { StageCard } from "@/components/StageCard";
import type { Stage } from "@/lib/types";

describe("StageCard date format", () => {
  it("exibe início e fim em padrão brasileiro com minuto", () => {
    const stage: Stage = {
      id: "1",
      file_id: "f1",
      stage: "download",
      status: "done",
      attempts: 1,
      started_at: "2026-04-28T13:14:15Z",
      finished_at: "2026-04-28T15:16:17Z",
      error_message: null,
      updated_at: "2026-04-28T15:16:17Z",
    };

    render(<StageCard stage={stage} />);

    const formatted = screen.getAllByText(/^\d{2}\/\d{2}\/\d{4} \d{2}:\d{2}$/);
    expect(formatted).toHaveLength(2);
  });
});
