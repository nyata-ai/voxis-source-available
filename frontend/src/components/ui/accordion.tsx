import * as React from "react"
import * as AccordionPrimitive from "@radix-ui/react-accordion"
import { ChevronDown } from "lucide-react"

import { cn } from "@/lib/utils"

const Accordion = AccordionPrimitive.Root

const AccordionItem = React.forwardRef<
  React.ElementRef<typeof AccordionPrimitive.Item>,
  React.ComponentPropsWithoutRef<typeof AccordionPrimitive.Item>
>(({ className, ...props }, ref) => (
  <AccordionPrimitive.Item ref={ref} className={cn("border-b", className)} {...props} />
))
AccordionItem.displayName = "AccordionItem"

/**
 * The heading row. Exported separately from the trigger — unlike stock shadcn,
 * which folds the header into the trigger — because a section header here has
 * to hold controls the trigger cannot contain: a Generate button inside a
 * button is invalid markup, and moving it out of the header would bury the
 * only control an empty section has behind a disclosure the user has no reason
 * to open.
 */
const AccordionHeader = React.forwardRef<
  React.ElementRef<typeof AccordionPrimitive.Header>,
  React.ComponentPropsWithoutRef<typeof AccordionPrimitive.Header>
>(({ className, ...props }, ref) => (
  <AccordionPrimitive.Header ref={ref} className={cn("flex", className)} {...props} />
))
AccordionHeader.displayName = "AccordionHeader"

const AccordionTrigger = React.forwardRef<
  React.ElementRef<typeof AccordionPrimitive.Trigger>,
  React.ComponentPropsWithoutRef<typeof AccordionPrimitive.Trigger>
>(({ className, children, ...props }, ref) => (
  <AccordionPrimitive.Trigger
    ref={ref}
    className={cn(
      "flex min-w-0 flex-1 items-center gap-2 py-3 text-left text-sm font-medium ring-offset-background transition-colors focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-offset-2 disabled:pointer-events-none disabled:opacity-50 [&[data-state=open]>svg.accordion-chevron]:rotate-0",
      className
    )}
    {...props}
  >
    {/* Chevron first, not last: a section header carries controls after the
        trigger, and a disclosure marker stranded between the label and those
        controls reads as belonging to them. */}
    <ChevronDown
      className="accordion-chevron h-4 w-4 shrink-0 -rotate-90 text-muted-foreground transition-transform duration-200"
      aria-hidden="true"
    />
    {children}
  </AccordionPrimitive.Trigger>
))
AccordionTrigger.displayName = AccordionPrimitive.Trigger.displayName

const AccordionContent = React.forwardRef<
  React.ElementRef<typeof AccordionPrimitive.Content>,
  React.ComponentPropsWithoutRef<typeof AccordionPrimitive.Content>
>(({ className, children, ...props }, ref) => (
  <AccordionPrimitive.Content
    ref={ref}
    className="overflow-hidden text-sm data-[state=closed]:animate-accordion-up data-[state=open]:animate-accordion-down"
    {...props}
  >
    <div className={cn("pb-4 pt-0", className)}>{children}</div>
  </AccordionPrimitive.Content>
))
AccordionContent.displayName = AccordionPrimitive.Content.displayName

export { Accordion, AccordionItem, AccordionHeader, AccordionTrigger, AccordionContent }
