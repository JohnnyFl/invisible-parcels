import type { Level, ProblemKind } from "../lib/api";

export const levelText: Record<Level, string> = {
  red: "Діяти зараз",
  orange: "Перевірити",
  yellow: "Спостерігати",
  grey: "Інцидент",
  green: "За планом",
};

export const problemText: Record<ProblemKind, string> = {
  unknown: "Невідома",
  misrouted: "Не за адресою",
  risky_return: "Ризикове повернення",
  broken_chain: "Розірваний ланцюг",
  overdue: "Прострочена",
  stuck: "Застигла",
  incident: "Інцидент",
};

/** What the operator should do, per problem — the row's call to action. */
export const problemHint: Record<ProblemKind, string> = {
  unknown: "Посилки немає в системі. Тримайте її на полиці ідентифікації, не повертайте навмання.",
  misrouted: "Посилка не з маршруту цього відділення. Переадресуйте її, щоб вона не загубилась.",
  risky_return: "Посилку повертають без ідентифікації. З'ясуйте, хто і чому її забрав.",
  broken_chain: "Рейс прибув, а вивантаження не відскановано. Перевірте полиці й зону розвантаження.",
  overdue: "Прогноз доставки минув, а посилка не прибула. Можливо, вона вже на складі без скану.",
  stuck: "Понад 12 годин жодних подій. Уточніть стан у сортувального центру.",
  incident: "Рейс постраждав. Затримка має відому причину, посилка не загублена.",
};

export const levelClass: Record<Level, { fg: string; bg: string; rail: string }> = {
  red: { fg: "text-lvl-red", bg: "bg-lvl-red-bg", rail: "bg-lvl-red" },
  orange: { fg: "text-lvl-orange", bg: "bg-lvl-orange-bg", rail: "bg-lvl-orange" },
  yellow: { fg: "text-lvl-yellow", bg: "bg-lvl-yellow-bg", rail: "bg-lvl-yellow" },
  grey: { fg: "text-lvl-grey", bg: "bg-lvl-grey-bg", rail: "bg-lvl-grey" },
  green: { fg: "text-lvl-green", bg: "bg-lvl-green-bg", rail: "bg-lvl-green" },
};

/** Shape differs per level, so the badge reads without colour too. */
function Icon({ level }: { level: Level }) {
  const common = { width: 14, height: 14, viewBox: "0 0 14 14", "aria-hidden": true } as const;
  switch (level) {
    case "red": // octagon with bar
      return (
        <svg {...common}>
          <path d="M4.1 1h5.8L13 4.1v5.8L9.9 13H4.1L1 9.9V4.1z" fill="currentColor" />
          <rect x="3.5" y="6.2" width="7" height="1.6" fill="#fff" />
        </svg>
      );
    case "orange": // triangle with mark
      return (
        <svg {...common}>
          <path d="M7 1l6.5 12H.5z" fill="currentColor" />
          <rect x="6.25" y="5" width="1.5" height="4" fill="#fff" />
          <rect x="6.25" y="10" width="1.5" height="1.5" fill="#fff" />
        </svg>
      );
    case "yellow": // ring
      return (
        <svg {...common}>
          <circle cx="7" cy="7" r="5" fill="none" stroke="currentColor" strokeWidth="2" />
        </svg>
      );
    case "grey": // bolt
      return (
        <svg {...common}>
          <path d="M8 1L2.5 8h3.5L5 13l6.5-7.5H8z" fill="currentColor" />
        </svg>
      );
    case "green": // check
      return (
        <svg {...common}>
          <path d="M2 7.5l3 3 7-7" fill="none" stroke="currentColor" strokeWidth="2" />
        </svg>
      );
  }
}

export function LevelBadge({ level, label }: { level: Level; label?: string }) {
  const c = levelClass[level];
  return (
    <span className={`inline-flex items-center gap-1.5 rounded px-2 py-0.5 text-sm font-medium ${c.fg} ${c.bg}`}>
      <Icon level={level} />
      {label ?? levelText[level]}
    </span>
  );
}
