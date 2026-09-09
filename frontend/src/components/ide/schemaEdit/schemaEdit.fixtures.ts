import type { SchemaEditSpec } from '#/lib/api/types'

export const editor: SchemaEditSpec = {
  operations: ['create_table', 'add_column', 'alter_column', 'create_index'],
  column_types: ['NUMBER', 'VARCHAR2(255)'],
  parameterized_column_types: [
    {
      name: 'NUMBER',
      parameters: [
        { name: 'precision', min: 1, max: 38 },
        { name: 'scale', min: -84, max: 127, optional: true },
      ],
    },
    { name: 'VARCHAR2', parameters: [{ name: 'length', min: 1, max: 4000 }] },
  ],
  supports_column_defaults: true,
  supports_cascade: true,
  creatable_table_scope_kinds: ['schema'],
  droppable_object_kinds: ['table'],
  droppable_scope_kinds: [],
}
