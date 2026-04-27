"use client";

import { useCallback } from "react";
import Link from "next/link";
import { DownloadPolicySection } from "@/components/DownloadPolicySection";

export default function PoliciesPage() {
  const onSaved = useCallback(() => {}, []);

  return (
    <main className="min-h-screen p-6">
      <div className="mx-auto max-w-7xl">
        <div className="mb-6 flex items-center justify-between">
          <div>
            <h1 className="text-3xl font-bold text-[var(--foreground)]">Política de processamento</h1>
            <p className="mt-1 text-sm text-[var(--muted)]">
              Defina etapas do pipeline e o escopo por catálogo e período.
            </p>
          </div>
          <Link href="/" className="secondary-link-chip">
            Voltar ao painel
          </Link>
        </div>
        <DownloadPolicySection onSaved={onSaved} />
      </div>
    </main>
  );
}
