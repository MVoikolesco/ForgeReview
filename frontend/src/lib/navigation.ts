import type { CurrentUser } from "./api";

export type NavigationItem = {
  href: string;
  label: string;
  roles?: CurrentUser["role"][];
};

export const navigationItems: NavigationItem[] = [
  { href: "/", label: "Visão geral" },
  { href: "/pipelines", label: "Pipelines" },
  { href: "/studio", label: "Studio", roles: ["editor", "admin"] },
  { href: "/integrations", label: "Integrações", roles: ["admin"] },
  { href: "/admin", label: "Administração", roles: ["admin"] },
];

export function visibleNavigation(role: CurrentUser["role"] | undefined) {
  return navigationItems.filter(
    (item) => !item.roles || (role && item.roles.includes(role)),
  );
}

export function breadcrumbs(pathname: string, current: string) {
  const item = navigationItems.find((entry) => entry.href === pathname);
  return item?.href === "/"
    ? [{ label: item.label }]
    : [
        { label: "Visão geral", href: "/" },
        { label: current || item?.label || "Área" },
      ];
}
