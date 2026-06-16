import { useCallback, useEffect, useRef, useState } from 'react'
import type { Command } from '@codemirror/view'
import type { EditorView } from '@codemirror/view'
import {
  SearchQuery,
  closeSearchPanel,
  findNext,
  findPrevious,
  getSearchQuery,
  replaceAll,
  replaceNext,
  setSearchQuery,
} from '@codemirror/search'
import { Button } from '#/components/ui/button'
import { Input } from '#/components/ui/input'
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from '#/components/ui/tooltip'
import { Icon } from '#/lib/icons'
import { cn } from '#/lib/utils'
import { computeMatchInfo, type MatchInfo } from './findMatches'

/** Wraps a control with a design-system tooltip without adding wrapper DOM. */
function Tip({ label, children }: { label: string; children: React.ReactElement }) {
  return (
    <Tooltip>
      <TooltipTrigger render={children} />
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  )
}

export function FindPanel({ view }: { view: EditorView }) {
  const initial = getSearchQuery(view.state)
  const [search, setSearch] = useState(initial.search)
  const [replace, setReplace] = useState(initial.replace)
  const [caseSensitive, setCaseSensitive] = useState(initial.caseSensitive)
  const [regexp, setRegexp] = useState(initial.regexp)
  const [wholeWord, setWholeWord] = useState(initial.wholeWord)
  const [match, setMatch] = useState<MatchInfo>(() => computeMatchInfo(view.state, initial))

  const searchRef = useRef<HTMLInputElement>(null)
  const readOnly = view.state.readOnly

  // Focus + select the query text when the panel opens.
  useEffect(() => {
    searchRef.current?.focus()
    searchRef.current?.select()
  }, [])

  // Recompute the counter from the live state — view.state is already updated
  // synchronously after dispatch / command execution.
  const recompute = useCallback(() => {
    setMatch(computeMatchInfo(view.state, getSearchQuery(view.state)))
  }, [view])

  const applyQuery = useCallback(
    (next: Partial<{ search: string; replace: string; caseSensitive: boolean; regexp: boolean; wholeWord: boolean }>) => {
      const query = new SearchQuery({
        search: next.search ?? search,
        replace: next.replace ?? replace,
        caseSensitive: next.caseSensitive ?? caseSensitive,
        regexp: next.regexp ?? regexp,
        wholeWord: next.wholeWord ?? wholeWord,
      })
      view.dispatch({ effects: setSearchQuery.of(query) })
      setMatch(computeMatchInfo(view.state, query))
    },
    [view, search, replace, caseSensitive, regexp, wholeWord],
  )

  const run = useCallback((cmd: Command) => { cmd(view); recompute() }, [view, recompute])

  function onSearchKeyDown(e: React.KeyboardEvent<HTMLInputElement>) {
    if (e.key === 'Enter') {
      e.preventDefault()
      run(e.shiftKey ? findPrevious : findNext)
    } else if (e.key === 'Escape') {
      e.preventDefault()
      closeSearchPanel(view)
    }
  }

  const counter = search === '' ? '' : match.total === 0 ? '0/0' : `${match.current}/${match.total}`

  return (
    <TooltipProvider delay={400}>
    <div className="flex flex-col gap-1 bg-popover px-2 py-1.5 text-xs text-popover-foreground">
      {/* Find row */}
      <div className="flex min-w-0 items-center gap-1">
        <Input
          ref={searchRef}
          value={search}
          onChange={(e) => { setSearch(e.target.value); applyQuery({ search: e.target.value }) }}
          onKeyDown={onSearchKeyDown}
          placeholder="Find"
          className="h-6 min-w-0 flex-1 px-2 text-xs"
        />

        <div className="flex shrink-0 items-center gap-0.5">
          <ToggleButton label="Aa" tip="Match case" active={caseSensitive} onClick={() => { const v = !caseSensitive; setCaseSensitive(v); applyQuery({ caseSensitive: v }) }} />
          <ToggleButton label=".*" tip="Use regular expression" active={regexp} onClick={() => { const v = !regexp; setRegexp(v); applyQuery({ regexp: v }) }} />
          <ToggleButton label="ab" tip="Match whole word" active={wholeWord} onClick={() => { const v = !wholeWord; setWholeWord(v); applyQuery({ wholeWord: v }) }} />
        </div>

        <span className="w-12 shrink-0 text-right tabular-nums text-muted-foreground">{counter}</span>

        <div className="flex shrink-0 items-center gap-0.5">
          <Tip label="Previous match (⇧⏎)">
            <Button type="button" variant="ghost" size="icon-sm" aria-label="Previous match" onClick={() => run(findPrevious)}>
              <Icon name="chevron-down" size={14} className="rotate-180" />
            </Button>
          </Tip>
          <Tip label="Next match (⏎)">
            <Button type="button" variant="ghost" size="icon-sm" aria-label="Next match" onClick={() => run(findNext)}>
              <Icon name="chevron-down" size={14} />
            </Button>
          </Tip>
          <Tip label="Close (Esc)">
            <Button type="button" variant="ghost" size="icon-sm" aria-label="Close find" onClick={() => closeSearchPanel(view)}>
              <Icon name="cancel-01" size={14} />
            </Button>
          </Tip>
        </div>
      </div>

      {/* Replace row */}
      {!readOnly && (
        <div className="flex min-w-0 items-center gap-1">
          <Input
            value={replace}
            onChange={(e) => { setReplace(e.target.value); applyQuery({ replace: e.target.value }) }}
            placeholder="Replace"
            className="h-6 min-w-0 flex-1 px-2 text-xs"
          />
          <Button type="button" variant="outline" size="sm" className="shrink-0" onClick={() => run(replaceNext)}>
            Replace
          </Button>
          <Button type="button" variant="outline" size="sm" className="shrink-0" onClick={() => run(replaceAll)}>
            Replace All
          </Button>
        </div>
      )}
    </div>
    </TooltipProvider>
  )
}

function ToggleButton({
  label,
  tip,
  active,
  onClick,
}: {
  label: string
  tip: string
  active: boolean
  onClick: () => void
}) {
  return (
    <Tip label={tip}>
      <Button
        type="button"
        variant={active ? 'secondary' : 'ghost'}
        size="icon-sm"
        aria-pressed={active}
        aria-label={tip}
        onClick={onClick}
        className={cn('font-mono text-[0.6875rem] leading-none', active && 'ring-1 ring-ring/50')}
      >
        {label}
      </Button>
    </Tip>
  )
}
