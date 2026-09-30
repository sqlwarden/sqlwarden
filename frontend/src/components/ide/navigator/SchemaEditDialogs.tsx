import type {
  DbColumn,
  NavigatorTree,
  ObjectRef,
  SchemaEditRequest,
  SchemaEditSpec,
  ScopePath,
  StatementOperation,
} from '#/lib/api/types'
import { scopeLabel } from '#/lib/api/scope'
import { GenerateStatementDialog } from '../GenerateStatementDialog'
import { ColumnEditDialog } from '../schemaEdit/ColumnEditDialog'
import { CreateIndexDialog } from '../schemaEdit/CreateIndexDialog'
import { CreateTableDialog } from '../schemaEdit/CreateTableDialog'
import { DropConfirmDialog } from '../schemaEdit/DropConfirmDialog'
import { RenameColumnDialog } from '../schemaEdit/RenameColumnDialog'
import { cascadeAvailable } from '../schemaEditCapability'
import { useSchemaEdit } from '../useSchemaEdit'

export type DropTarget =
  | { kind: 'scope'; scope: ScopePath; scopeKind: string }
  | { kind: 'object'; ref: ObjectRef }
  | { kind: 'column'; ref: ObjectRef; columnName: string }
  | { kind: 'index'; ref: ObjectRef; indexName: string }

export type EditTarget =
  | { type: 'create_table'; scope: ScopePath }
  | { type: 'column'; ref: ObjectRef; column?: DbColumn }
  | { type: 'index'; ref: ObjectRef }
  | { type: 'rename_column'; ref: ObjectRef; columnName: string }
  | { type: 'drop'; target: DropTarget }
  | { type: 'generate'; ref: ObjectRef; operation: StatementOperation }

const kindLabel = (tree: NavigatorTree, kind: string) =>
  (tree.nodes[kind]?.label ?? kind).toLowerCase()

function dropTargetLabel(target: DropTarget, tree: NavigatorTree): string {
  switch (target.kind) {
    case 'scope':
      return kindLabel(tree, target.scopeKind)
    case 'object':
      return kindLabel(tree, target.ref.kind)
    case 'column':
      return 'column'
    case 'index':
      return 'index'
  }
}

function dropTargetName(target: DropTarget): string {
  switch (target.kind) {
    case 'scope':
      return scopeLabel(target.scope)
    case 'object':
      return target.ref.name
    case 'column':
      return target.columnName
    case 'index':
      return target.indexName
  }
}

function dropTargetDescription(target: DropTarget, tree: NavigatorTree): string {
  switch (target.kind) {
    case 'scope':
      return `This permanently drops ${scopeLabel(target.scope)} and everything it contains. This cannot be undone.`
    case 'object':
      return `This permanently drops the ${kindLabel(tree, target.ref.kind)} ${target.ref.name}. This cannot be undone.`
    case 'column':
      return `This permanently drops the column ${target.columnName} from ${target.ref.name}, including its data. This cannot be undone.`
    case 'index':
      return `This permanently drops the index ${target.indexName} from ${target.ref.name}. This cannot be undone.`
  }
}

function dropTargetRequest(target: DropTarget, cascade: boolean): SchemaEditRequest {
  switch (target.kind) {
    case 'scope':
      return { operation: 'drop_scope', scope: target.scope, cascade }
    case 'object':
      return { operation: 'drop_object', ref: target.ref, cascade }
    case 'column':
      return { operation: 'drop_column', ref: target.ref, name: target.columnName, cascade }
    case 'index':
      return { operation: 'drop_index', ref: target.ref, name: target.indexName, cascade }
  }
}

export function SchemaEditDialogs({
  orgSlug,
  workspaceId,
  connectionId,
  sessionId,
  tree,
  editor,
  target,
  onClose,
}: {
  orgSlug: string
  workspaceId: number
  connectionId: number
  sessionId?: string
  tree: NavigatorTree
  editor: SchemaEditSpec | undefined
  target: EditTarget | null
  onClose: () => void
}) {
  const schemaEdit = useSchemaEdit({ orgSlug, workspaceId, connectionId, sessionId })
  const submit = (request: SchemaEditRequest) => schemaEdit.mutate(request, { onSuccess: onClose })

  return (
    <>
      {editor && (
        <CreateTableDialog
          open={target?.type === 'create_table'}
          onOpenChange={(open) => !open && onClose()}
          scope={target?.type === 'create_table' ? target.scope : []}
          columnTypes={editor.column_types}
          parameterizedColumnTypes={editor.parameterized_column_types}
          allowCustomColumnTypes={editor.allow_custom_column_types}
          supportsColumnDefaults={editor.supports_column_defaults}
          pending={schemaEdit.isPending}
          onSubmit={(name, columns) =>
            target?.type === 'create_table' &&
            submit({ operation: 'create_table', scope: target.scope, name, columns })
          }
        />
      )}
      {target?.type === 'column' && editor && (
        <ColumnEditDialog
          objectRef={target.ref}
          column={target.column}
          editor={editor}
          pending={schemaEdit.isPending}
          onClose={onClose}
          onSubmit={submit}
        />
      )}
      {target?.type === 'index' && (
        <CreateIndexDialog
          objectRef={target.ref}
          orgSlug={orgSlug}
          workspaceId={workspaceId}
          connectionId={connectionId}
          sessionId={sessionId}
          pending={schemaEdit.isPending}
          onClose={onClose}
          onSubmit={submit}
        />
      )}
      {target?.type === 'rename_column' && (
        <RenameColumnDialog
          open={true}
          onOpenChange={(open) => !open && onClose()}
          tableName={target.ref.name}
          columnName={target.columnName}
          pending={schemaEdit.isPending}
          onSubmit={(newName) =>
            submit({
              operation: 'rename_column',
              ref: target.ref,
              name: target.columnName,
              new_name: newName,
            })
          }
        />
      )}
      {target?.type === 'drop' && (
        <DropConfirmDialog
          open={true}
          onOpenChange={(open) => !open && onClose()}
          objectType={dropTargetLabel(target.target, tree)}
          objectName={dropTargetName(target.target)}
          description={dropTargetDescription(target.target, tree)}
          cascadeAvailable={cascadeAvailable(editor, target.target.kind)}
          pending={schemaEdit.isPending}
          onConfirm={(cascade) => submit(dropTargetRequest(target.target, cascade))}
        />
      )}
      <GenerateStatementDialog
        open={target?.type === 'generate'}
        onOpenChange={(open) => !open && onClose()}
        orgSlug={orgSlug}
        workspaceId={workspaceId}
        connectionId={connectionId}
        sessionId={sessionId}
        target={
          target?.type === 'generate' ? { ref: target.ref, operation: target.operation } : null
        }
      />
    </>
  )
}
