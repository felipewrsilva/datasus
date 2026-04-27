import type { Metadata } from "next";
import Link from "next/link";
import "./globals.css";
import { ThemeToggle } from "@/components/ThemeToggle";

export const metadata: Metadata = {
  title: "Pipeline DATASUS",
  description: "Painel de processamento e conversão de arquivos do DATASUS",
};

export default function RootLayout({
  children,
}: Readonly<{
  children: React.ReactNode;
}>) {
  return (
    <html
      lang="pt-BR"
      className="h-full antialiased"
    >
      <body className="app-shell min-h-full flex flex-col">
        <header className="sticky top-0 z-20 border-b border-[var(--border)] bg-[var(--background)]/80 backdrop-blur">
          <div className="mx-auto flex w-full max-w-7xl items-center justify-between px-6 py-3">
            <Link
              href="/"
              className="rounded-md px-2 py-1 text-sm font-semibold tracking-wide text-[var(--foreground)] transition-colors hover:text-[var(--accent)] focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-[var(--accent)] focus-visible:ring-offset-2 focus-visible:ring-offset-[var(--background)]"
            >
              DATASUS
            </Link>
            <div className="flex items-center gap-2">
              <nav className="hidden items-center gap-2 sm:flex">
                <Link
                  href="/"
                  className="secondary-link-chip"
                >
                  Painel
                </Link>
                <Link
                  href="/files"
                  className="secondary-link-chip"
                >
                  Arquivos
                </Link>
                <Link
                  href="/policies"
                  className="secondary-link-chip"
                >
                  Políticas
                </Link>
              </nav>
              <ThemeToggle />
            </div>
          </div>
        </header>
        {children}
      </body>
    </html>
  );
}
