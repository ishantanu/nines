import * as React from "react";
import {
  Table as AstryxTable,
  TableBody as AstryxTableBody,
  TableCell as AstryxTableCell,
  TableFooter as AstryxTableFooter,
  TableHeader as AstryxTableHeader,
  TableHeaderCell as AstryxTableHeaderCell,
  TableRow as AstryxTableRow,
} from "@astryxdesign/core/Table";
import { cn } from "cn";

// Composable table parts backed by Astryx's children-mode Table.
type P<T extends keyof React.JSX.IntrinsicElements> = React.ComponentProps<T>;

function Table({ className, children, ...props }: P<"table">) {
  return (
    <div
      data-slot="table-container"
      className="relative w-full overflow-x-auto"
    >
      <AstryxTable
        data-slot="table"
        className={cn("w-full", className)}
        {...(props as object)}
      >
        {children}
      </AstryxTable>
    </div>
  );
}
function TableHeader({ className, children, ...props }: P<"thead">) {
  return (
    <AstryxTableHeader
      data-slot="table-header"
      className={className}
      {...(props as object)}
    >
      {children}
    </AstryxTableHeader>
  );
}
function TableBody({ className, children, ...props }: P<"tbody">) {
  return (
    <AstryxTableBody
      data-slot="table-body"
      className={className}
      {...(props as object)}
    >
      {children}
    </AstryxTableBody>
  );
}
function TableFooter({ className, children, ...props }: P<"tfoot">) {
  return (
    <AstryxTableFooter
      data-slot="table-footer"
      className={className}
      {...(props as object)}
    >
      {children}
    </AstryxTableFooter>
  );
}
function TableRow({ className, children, ...props }: P<"tr">) {
  return (
    <AstryxTableRow
      data-slot="table-row"
      className={className}
      {...(props as object)}
    >
      {children}
    </AstryxTableRow>
  );
}
function TableHead({ className, ...props }: P<"th">) {
  return (
    <AstryxTableHeaderCell
      data-slot="table-head"
      className={className}
      {...(props as object)}
    />
  );
}
function TableCell({ className, ...props }: P<"td">) {
  return (
    <AstryxTableCell
      data-slot="table-cell"
      className={className}
      {...(props as object)}
    />
  );
}
function TableCaption({ className, ...props }: P<"caption">) {
  return (
    <caption
      data-slot="table-caption"
      className={cn("mt-4 text-sm text-muted-foreground", className)}
      {...props}
    />
  );
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
};
