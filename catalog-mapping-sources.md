# Catalog mapping sources

This file tracks public sources used to improve catalog label mapping.

## Sources checked

- DATASUS download area, [Download TabWin](http://datasus1.saude.gov.br/transferencia-download-de-arquivos/download-do-tabwin)
- DATASUS educational material, [Curso de Tabulação Básica com TABWIN](http://universus3.datasus.gov.br/universus/tabwin/unidade1/tema3/unid1_tema3_tela6.php)
- Community usage guide, [Como usar as bases de dados do DATASUS](https://mobilidadeativa.org.br/como-usar-as-bases-do-datasus/)

## Current status

- Existing app labels still use the curated local mapping in `web/src/lib/catalogLabels.ts`.
- Unknown catalog codes now render as `XX - Catálogo não mapeado`.
- Next step is to automate extraction from DATASUS/TABWIN catalog definitions into a versioned mapping artifact.
