import { Button as ButtonPrimitive } from "@base-ui/react/button"
import { cva, type VariantProps } from "class-variance-authority"
import { cn } from "cn"

const buttonVariants = cva(
  "group/button inline-flex shrink-0 items-center justify-center rounded-md border border-transparent bg-clip-padding text-ui font-medium whitespace-nowrap transition-colors duration-100 outline-none select-none focus-visible:ring-2 focus-visible:ring-ring/50 disabled:pointer-events-none disabled:opacity-50 aria-invalid:border-destructive [&_svg]:pointer-events-none [&_svg]:shrink-0 [&_svg:not([class*='size-'])]:size-4",
  {
    variants: {
      variant: {
        default: "bg-brand text-white shadow-[inset_0_1px_0_rgb(255_255_255/0.12),0_1px_2px_rgb(0_0_0/0.12)] hover:bg-[color-mix(in_oklch,var(--brand),white_8%)]",
        outline:
          "border-border bg-background text-foreground shadow-xs hover:bg-accent aria-expanded:bg-accent dark:bg-white/[0.03] dark:hover:bg-white/[0.06]",
        secondary: "bg-secondary text-secondary-foreground hover:bg-accent",
        ghost: "text-muted-foreground hover:bg-accent hover:text-foreground aria-expanded:bg-accent aria-expanded:text-foreground",
        destructive: "bg-destructive/10 text-destructive hover:bg-destructive/15 dark:bg-destructive/20 dark:hover:bg-destructive/25",
        link: "text-brand underline-offset-4 hover:underline",
      },
      size: {
        default: "h-9 gap-2 px-3.5",
        xs: "h-7 gap-1 px-2.5 text-mini [&_svg:not([class*='size-'])]:size-3.5",
        sm: "h-8 gap-1.5 px-3",
        lg: "h-9 gap-2 px-4",
        icon: "size-9",
        "icon-xs": "size-7 [&_svg:not([class*='size-'])]:size-3.5",
        "icon-sm": "size-8",
        "icon-lg": "size-9",
      },
    },
    defaultVariants: {
      variant: "default",
      size: "default",
    },
  }
)

function Button({
  className,
  variant = "default",
  size = "default",
  ...props
}: ButtonPrimitive.Props & VariantProps<typeof buttonVariants>) {
  return (
    <ButtonPrimitive
      data-slot="button"
      className={cn(buttonVariants({ variant, size, className }))}
      {...props}
    />
  )
}

export { Button, buttonVariants }
