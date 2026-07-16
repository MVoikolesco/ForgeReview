export type Row = Record<string, any>;
export type SelectOption = { value: string | number; label: string };
export type FieldReference = {
  path: string;
  labelKeys: string[];
  placeholder?: string;
};
export type Field = {
  key: string;
  label: string;
  type?: "text" | "number" | "boolean" | "textarea";
  required?: boolean;
  default?: unknown;
  hint?: string;
  options?: SelectOption[];
  reference?: FieldReference;
};
export type ResourceGroup = "IA" | "Review" | "Integrações";
export type Resource = {
  name: string;
  singular: string;
  path: string;
  description: string;
  icon: string;
  group: ResourceGroup;
  fields: Field[];
  seeded?: boolean;
  canDefault?: boolean;
  canTest?: boolean;
};
