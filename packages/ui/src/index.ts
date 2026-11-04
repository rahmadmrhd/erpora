// Barrel `@erpora/ui`: konsumen hanya mengimpor dari sini (aturan F2).
// Komponen `components/ui/*` = hasil `shadcn add` (persis registry; jangan diubah tangan, lihat shadcn.lock.json);
// `components/extended/*` = pembungkus tipis untuk kebutuhan DS tanpa padanan shadcn. `Toaster` ada di entry sekunder `@erpora/ui/toaster`.

export { LoadingButton } from "./components/extended/loading-button";
export { StatusBadge, type StatusTone } from "./components/extended/status-badge";
export { Alert, AlertDescription, AlertTitle } from "./components/ui/alert";
export {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogMedia,
  AlertDialogOverlay,
  AlertDialogPortal,
  AlertDialogTitle,
  AlertDialogTrigger,
} from "./components/ui/alert-dialog";
export {
  Avatar,
  AvatarBadge,
  AvatarFallback,
  AvatarGroup,
  AvatarGroupCount,
  AvatarImage,
} from "./components/ui/avatar";
export { Badge, badgeVariants } from "./components/ui/badge";
export {
  Breadcrumb,
  BreadcrumbEllipsis,
  BreadcrumbItem,
  BreadcrumbLink,
  BreadcrumbList,
  BreadcrumbPage,
  BreadcrumbSeparator,
} from "./components/ui/breadcrumb";
export { Button, buttonVariants } from "./components/ui/button";
export {
  Card,
  CardAction,
  CardContent,
  CardDescription,
  CardFooter,
  CardHeader,
  CardTitle,
} from "./components/ui/card";
export { Checkbox } from "./components/ui/checkbox";
export { Collapsible, CollapsibleContent, CollapsibleTrigger } from "./components/ui/collapsible";
export {
  Dialog,
  DialogClose,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogOverlay,
  DialogPortal,
  DialogTitle,
  DialogTrigger,
} from "./components/ui/dialog";
export {
  DropdownMenu,
  DropdownMenuCheckboxItem,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuPortal,
  DropdownMenuRadioGroup,
  DropdownMenuRadioItem,
  DropdownMenuSeparator,
  DropdownMenuShortcut,
  DropdownMenuSub,
  DropdownMenuSubContent,
  DropdownMenuSubTrigger,
  DropdownMenuTrigger,
} from "./components/ui/dropdown-menu";
export {
  Empty,
  EmptyContent,
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "./components/ui/empty";
export { Input } from "./components/ui/input";
export {
  InputOTP,
  InputOTPGroup,
  InputOTPSeparator,
  InputOTPSlot,
} from "./components/ui/input-otp";
export { Kbd, KbdGroup } from "./components/ui/kbd";
export { Label } from "./components/ui/label";
export {
  Popover,
  PopoverAnchor,
  PopoverContent,
  PopoverDescription,
  PopoverHeader,
  PopoverTitle,
  PopoverTrigger,
} from "./components/ui/popover";
export { RadioGroup, RadioGroupItem } from "./components/ui/radio-group";
export {
  Select,
  SelectContent,
  SelectGroup,
  SelectItem,
  SelectLabel,
  SelectScrollDownButton,
  SelectScrollUpButton,
  SelectSeparator,
  SelectTrigger,
  SelectValue,
} from "./components/ui/select";
export { Separator } from "./components/ui/separator";
export {
  Sheet,
  SheetClose,
  SheetContent,
  SheetDescription,
  SheetFooter,
  SheetHeader,
  SheetTitle,
  SheetTrigger,
} from "./components/ui/sheet";
export {
  Sidebar,
  SidebarContent,
  SidebarFooter,
  SidebarGroup,
  SidebarGroupAction,
  SidebarGroupContent,
  SidebarGroupLabel,
  SidebarHeader,
  SidebarInput,
  SidebarInset,
  SidebarMenu,
  SidebarMenuAction,
  SidebarMenuBadge,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarMenuSkeleton,
  SidebarMenuSub,
  SidebarMenuSubButton,
  SidebarMenuSubItem,
  SidebarProvider,
  SidebarRail,
  SidebarSeparator,
  SidebarTrigger,
  useSidebar,
} from "./components/ui/sidebar";
export { Skeleton } from "./components/ui/skeleton";
export { Spinner } from "./components/ui/spinner";
export {
  Table,
  TableBody,
  TableCaption,
  TableCell,
  TableFooter,
  TableHead,
  TableHeader,
  TableRow,
} from "./components/ui/table";
export { Tabs, TabsContent, TabsList, TabsTrigger, tabsListVariants } from "./components/ui/tabs";
export { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "./components/ui/tooltip";
export { useIsMobile } from "./hooks/use-mobile";
export { type IconName, iconRegistry, resolveIcon } from "./icons";
export {
  type UiKey,
  UiLabel,
  type UiLabelFn,
  UiLabelsProvider,
  uiKeys,
  useUiLabel,
} from "./labels";
export {
  type FitInput,
  type FitPlan,
  type FitSlot,
  type FitStage,
  fitBreadcrumb,
} from "./lib/breadcrumb-fit";
export { initialsOf } from "./lib/initials";
export { cn } from "./lib/utils";
