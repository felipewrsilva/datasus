const CATALOG_NAMES: Record<string, string> = {
  AD: "APAC de Laudos Diversos",
  RD: "AIH Reduzida",
  SP: "Serviços Profissionais",
  AQ: "APAC de Quimioterapia",
  AM: "APAC de Medicamentos",
  BI: "Boletim de Produção Ambulatorial Individualizado",
  RJ: "AIH Rejeitada",
  PA: "Produção Ambulatorial",
  AR: "APAC de Radioterapia",
  ER: "AIH Rejeitada com código de Erro",
  PS: "RAAS Psicossocial",
  AN: "APAC de Nefrologia",
};

export function catalogNamePtBR(code: string): string | null {
  const normalized = code.trim().toUpperCase();
  return CATALOG_NAMES[normalized] ?? null;
}

export function formatCatalogLabel(code: string): string {
  const normalized = code.trim().toUpperCase();
  const name = catalogNamePtBR(normalized);
  return name ? `${normalized} - ${name}` : `${normalized} - Catálogo não mapeado`;
}
