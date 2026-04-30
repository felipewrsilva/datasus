export const UF_TO_PTBR: Record<string, string> = {
  AC: "Acre",
  AL: "Alagoas",
  AP: "Amapa",
  AM: "Amazonas",
  BA: "Bahia",
  CE: "Ceara",
  DF: "Distrito Federal",
  ES: "Espirito Santo",
  GO: "Goias",
  MA: "Maranhao",
  MT: "Mato Grosso",
  MS: "Mato Grosso do Sul",
  MG: "Minas Gerais",
  PA: "Para",
  PB: "Paraiba",
  PR: "Parana",
  PE: "Pernambuco",
  PI: "Piaui",
  RJ: "Rio de Janeiro",
  RN: "Rio Grande do Norte",
  RS: "Rio Grande do Sul",
  RO: "Rondonia",
  RR: "Roraima",
  SC: "Santa Catarina",
  SP: "Sao Paulo",
  SE: "Sergipe",
  TO: "Tocantins",
};

export const POLICY_STATES = Object.entries(UF_TO_PTBR)
  .map(([uf, name]) => ({ uf, name, label: `${uf} - ${name}` }))
  .sort((a, b) => a.uf.localeCompare(b.uf));

export function stateNamePtBR(uf: string | null | undefined): string {
  const key = (uf ?? "").trim().toUpperCase();
  if (!key) return "UF desconhecida";
  return UF_TO_PTBR[key] ?? `UF desconhecida (${key})`;
}
