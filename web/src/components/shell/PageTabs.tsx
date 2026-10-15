import { TabsList, TabsTrigger } from "@/components/ui/tabs";

// PageTabs are the views of a page, small, beside its path in the header.
export function PageTabs({ tabs }: { tabs: { value: string; label: string; count?: number }[] }) {
  return (
    <TabsList variant="line" className="flex h-8 max-w-full justify-start gap-0.5 overflow-x-auto p-0 [scrollbar-width:none]">
      {tabs.map((tab) => (
        <TabsTrigger
          key={tab.value}
          value={tab.value}
          className="h-8 flex-none rounded-md border-0 px-2.5 text-ui! font-medium text-muted-foreground after:hidden hover:bg-accent/60 hover:text-foreground data-active:bg-accent! data-active:text-foreground!"
        >
          {tab.label}
          {tab.count ? <span className="text-mini tabular-nums text-muted-foreground">{tab.count}</span> : null}
        </TabsTrigger>
      ))}
    </TabsList>
  );
}
