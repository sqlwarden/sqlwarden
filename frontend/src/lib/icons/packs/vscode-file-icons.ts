export type FileTypeIconName =
  | 'sql'
  | 'csv'
  | 'json'
  | 'xml'
  | 'yaml'
  | 'toml'
  | 'markdown'
  | 'parquet'
  | 'excel'
  | 'shell'
  | 'text'
  | 'log'
  | 'binary'
  | 'audio'
  | 'default'

const icons: Record<FileTypeIconName, string> = {
  sql: 'vscode-icons:file-type-sql',
  csv: 'vscode-icons:file-type-text',
  json: 'vscode-icons:file-type-json',
  xml: 'vscode-icons:file-type-xml',
  yaml: 'vscode-icons:file-type-yaml',
  toml: 'vscode-icons:file-type-toml',
  markdown: 'vscode-icons:file-type-markdown',
  parquet: 'vscode-icons:file-type-parquet',
  excel: 'vscode-icons:file-type-excel',
  shell: 'vscode-icons:file-type-shell',
  text: 'vscode-icons:file-type-text',
  log: 'vscode-icons:file-type-log',
  binary: 'vscode-icons:file-type-binary',
  audio: 'vscode-icons:file-type-audio',
  default: 'vscode-icons:default-file',
}

export default icons
