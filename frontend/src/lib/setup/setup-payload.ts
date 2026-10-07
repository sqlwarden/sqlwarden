export type SetupFormValues = {
  name: string
  email: string
  password: string
  organization_name: string
  organization_slug: string
}

export function buildSetupPayload(
  values: SetupFormValues,
  requiresInput: boolean,
): SetupFormValues | Record<string, never> {
  return requiresInput ? values : {}
}
