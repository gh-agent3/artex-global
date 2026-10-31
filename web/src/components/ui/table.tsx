"use client"

import * as React from "react"

import { getLocalStorageValue, setLocalStorageValue } from "@/lib/local-storage.client"
import { cn } from "@/lib/utils"

const MIN_COLUMN_WIDTH = 48

type ColumnWidths = Record<number, number>

type TableResizeContextValue = {
  widths: ColumnWidths
  resized: boolean
  label: string
  onPointerDown: (event: React.PointerEvent<HTMLSpanElement>, index: number) => void
  onPointerMove: (event: React.PointerEvent<HTMLSpanElement>) => void
  onPointerUp: (event: React.PointerEvent<HTMLSpanElement>) => void
  onKeyDown: (event: React.KeyboardEvent<HTMLSpanElement>, index: number) => void
}

const TableResizeContext = React.createContext<TableResizeContextValue | null>(null)

function readColumnWidths(storageKey: string): ColumnWidths {
  const raw = getLocalStorageValue(storageKey)
  if (!raw) return {}
  try {
    const parsed: unknown = JSON.parse(raw)
    if (!parsed || typeof parsed !== "object" || Array.isArray(parsed)) return {}
    const widths: ColumnWidths = {}
    for (const [index, width] of Object.entries(parsed)) {
      if (/^\d+$/.test(index) && typeof width === "number" && Number.isFinite(width) && width >= MIN_COLUMN_WIDTH) {
        widths[Number(index)] = width
      }
    }
    return widths
  } catch {
    return {}
  }
}

function measureColumnWidths(table: HTMLTableElement, current: ColumnWidths): ColumnWidths {
  const widths = { ...current }
  const cells = table.tHead?.rows[0]?.cells
  if (!cells) return widths
  for (const cell of Array.from(cells)) {
    if (window.getComputedStyle(cell).display === "none") continue
    const width = cell.getBoundingClientRect().width
    if (width > 0) widths[cell.cellIndex] = Math.max(MIN_COLUMN_WIDTH, Math.round(width))
  }
  return widths
}

type TableProps = React.ComponentProps<"table"> & {
  resizeStorageKey?: string
  resizeLabel?: string
}

function Table({ className, style, resizeStorageKey, resizeLabel, ...props }: TableProps) {
  const [columnWidths, setColumnWidths] = React.useState<ColumnWidths>({})
  const widthsRef = React.useRef<ColumnWidths>({})
  const dragRef = React.useRef<{
    pointerId: number
    index: number
    startX: number
    startWidth: number
    widths: ColumnWidths
  } | null>(null)
  const resizable = Boolean(resizeStorageKey && resizeLabel)
  const resized = resizable && Object.keys(columnWidths).length > 0

  React.useEffect(() => {
    if (!resizeStorageKey) return
    const widths = readColumnWidths(resizeStorageKey)
    widthsRef.current = widths
    setColumnWidths(widths)
  }, [resizeStorageKey])

  const onPointerDown = React.useCallback((event: React.PointerEvent<HTMLSpanElement>, index: number) => {
    if (event.button !== 0) return
    const table = event.currentTarget.closest("table")
    if (!table) return
    event.preventDefault()
    event.stopPropagation()
    const widths = measureColumnWidths(table, widthsRef.current)
    widthsRef.current = widths
    setColumnWidths(widths)
    dragRef.current = {
      pointerId: event.pointerId,
      index,
      startX: event.clientX,
      startWidth: widths[index] ?? MIN_COLUMN_WIDTH,
      widths,
    }
    event.currentTarget.setPointerCapture(event.pointerId)
  }, [])

  const onPointerMove = React.useCallback((event: React.PointerEvent<HTMLSpanElement>) => {
    const drag = dragRef.current
    if (!drag || drag.pointerId !== event.pointerId) return
    event.preventDefault()
    const width = Math.max(MIN_COLUMN_WIDTH, Math.round(drag.startWidth + event.clientX - drag.startX))
    const widths = { ...drag.widths, [drag.index]: width }
    widthsRef.current = widths
    setColumnWidths(widths)
  }, [])

  const onPointerUp = React.useCallback(
    (event: React.PointerEvent<HTMLSpanElement>) => {
      const drag = dragRef.current
      if (!drag || drag.pointerId !== event.pointerId) return
      dragRef.current = null
      if (resizeStorageKey) setLocalStorageValue(resizeStorageKey, JSON.stringify(widthsRef.current))
      if (event.currentTarget.hasPointerCapture(event.pointerId)) {
        event.currentTarget.releasePointerCapture(event.pointerId)
      }
    },
    [resizeStorageKey],
  )

  const onKeyDown = React.useCallback(
    (event: React.KeyboardEvent<HTMLSpanElement>, index: number) => {
      if (event.key !== "ArrowLeft" && event.key !== "ArrowRight") return
      const table = event.currentTarget.closest("table")
      if (!table) return
      event.preventDefault()
      event.stopPropagation()
      const measured = measureColumnWidths(table, widthsRef.current)
      const delta = event.key === "ArrowRight" ? 8 : -8
      const widths = {
        ...measured,
        [index]: Math.max(MIN_COLUMN_WIDTH, Math.round((measured[index] ?? MIN_COLUMN_WIDTH) + delta)),
      }
      widthsRef.current = widths
      setColumnWidths(widths)
      if (resizeStorageKey) setLocalStorageValue(resizeStorageKey, JSON.stringify(widths))
    },
    [resizeStorageKey],
  )

  const resizeContext = React.useMemo<TableResizeContextValue | null>(
    () =>
      resizable && resizeLabel
        ? { widths: columnWidths, resized, label: resizeLabel, onPointerDown, onPointerMove, onPointerUp, onKeyDown }
        : null,
    [columnWidths, onKeyDown, onPointerDown, onPointerMove, onPointerUp, resizable, resizeLabel, resized],
  )

  // table-layout:fixed honours the <th> widths we set, but ONLY when the table
  // width itself is explicit: with width:max-content the browser sizes the table
  // to the sum of the headers' content widths and redistributes the slack, so a
  // dragged column snaps back to the width of its label text. Pinning the table
  // to the sum of the measured column widths makes the drag stick.
  const tableWidth = React.useMemo(() => {
    if (!resized) return undefined
    let total = 0
    for (const w of Object.values(columnWidths)) total += w
    return total > 0 ? total : undefined
  }, [columnWidths, resized])

  return (
    <div
      data-slot="table-container"
      className="relative w-full overflow-x-auto"
    >
      <TableResizeContext.Provider value={resizeContext}>
        <table
          data-slot="table"
          className={cn("w-full caption-bottom text-sm", className)}
          style={resized ? { ...style, tableLayout: "fixed", width: tableWidth, minWidth: "100%" } : style}
          {...props}
        />
      </TableResizeContext.Provider>
    </div>
  )
}

function TableHeader({ className, ...props }: React.ComponentProps<"thead">) {
  return (
    <thead
      data-slot="table-header"
      className={cn("[&_tr]:border-b", className)}
      {...props}
    />
  )
}

function TableBody({ className, ...props }: React.ComponentProps<"tbody">) {
  return (
    <tbody
      data-slot="table-body"
      className={cn("[&_tr:last-child]:border-0", className)}
      {...props}
    />
  )
}

function TableFooter({ className, ...props }: React.ComponentProps<"tfoot">) {
  return (
    <tfoot
      data-slot="table-footer"
      className={cn(
        "border-t bg-muted/50 font-medium [&>tr]:last:border-b-0",
        className
      )}
      {...props}
    />
  )
}

function TableRow({ className, ...props }: React.ComponentProps<"tr">) {
  return (
    <tr
      data-slot="table-row"
      className={cn(
        "border-b transition-colors hover:bg-muted/50 has-aria-expanded:bg-muted/50 data-[state=selected]:bg-muted",
        className
      )}
      {...props}
    />
  )
}

function TableHead({ className, style, children, ref, ...props }: React.ComponentProps<"th">) {
  const resize = React.useContext(TableResizeContext)
  const [columnIndex, setColumnIndex] = React.useState(-1)
  const setHeadRef = React.useCallback(
    (node: HTMLTableCellElement | null) => {
      if (node) setColumnIndex(node.cellIndex)
      if (typeof ref === "function") ref(node)
      else if (ref) ref.current = node
    },
    [ref],
  )
  const width = columnIndex >= 0 ? resize?.widths[columnIndex] : undefined

  return (
    <th
      data-slot="table-head"
      ref={setHeadRef}
      className={cn(
        "h-10 px-2 text-left align-middle font-medium whitespace-nowrap text-foreground [&:has([role=checkbox])]:pr-0",
        resize && "relative",
        resize?.resized && "overflow-hidden",
        className
      )}
      style={width === undefined ? style : { ...style, width }}
      {...props}
    >
      {children}
      {resize && columnIndex >= 0 && (
        <span
          data-slot="table-column-resizer"
          role="separator"
          aria-label={resize.label}
          aria-orientation="vertical"
          aria-valuemin={MIN_COLUMN_WIDTH}
          aria-valuenow={width === undefined ? undefined : Math.round(width)}
          title={resize.label}
          tabIndex={0}
          className="absolute inset-y-0 right-0 z-10 w-2 cursor-col-resize touch-none select-none after:absolute after:inset-y-1 after:right-0 after:w-px after:bg-primary after:opacity-0 after:transition-opacity hover:after:opacity-100 focus-visible:after:opacity-100"
          onPointerDown={(event) => resize.onPointerDown(event, columnIndex)}
          onPointerMove={resize.onPointerMove}
          onPointerUp={resize.onPointerUp}
          onPointerCancel={resize.onPointerUp}
          onLostPointerCapture={resize.onPointerUp}
          onKeyDown={(event) => resize.onKeyDown(event, columnIndex)}
          onClick={(event) => event.stopPropagation()}
        />
      )}
    </th>
  )
}

function TableCell({ className, ...props }: React.ComponentProps<"td">) {
  const resize = React.useContext(TableResizeContext)
  return (
    <td
      data-slot="table-cell"
      className={cn(
        "p-2 align-middle whitespace-nowrap [&:has([role=checkbox])]:pr-0",
        resize?.resized && "overflow-hidden",
        className
      )}
      {...props}
    />
  )
}

function TableCaption({
  className,
  ...props
}: React.ComponentProps<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  )
}

export {
  Table,
  TableHeader,
  TableBody,
  TableFooter,
  TableHead,
  TableRow,
  TableCell,
  TableCaption,
}
