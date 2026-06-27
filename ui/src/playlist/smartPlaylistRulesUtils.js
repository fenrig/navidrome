export const helperFields = [
  'smartMode',
  'smartLovedOnly',
  'smartTitle',
  'smartGenre',
  'smartArtist',
  'smartBpmPreset',
  'smartBpmMin',
  'smartBpmMax',
  'smartTagField',
  'smartTagValue',
  'smartSort',
  'smartOrder',
  'smartLimit',
]

const isPresent = (value) => value !== undefined && value !== null && value !== ''

const splitTerms = (value) =>
  isPresent(value)
    ? value
        .split(',')
        .map((term) => term.trim())
        .filter(Boolean)
    : []

const getSingleValue = (rule, operator, field) => {
  const value = rule?.[operator]?.[field]
  return isPresent(value) ? value : undefined
}

const getRuleValue = (rules, operator, field) => {
  for (const rule of rules || []) {
    const value = getSingleValue(rule, operator, field)
    if (value !== undefined) {
      return value
    }
  }
  return undefined
}

const getCustomTagRule = (rules) => {
  const knownFields = ['loved', 'title', 'genre', 'artist', 'bpm', 'playcount']
  for (const rule of rules || []) {
    const contains = rule?.contains
    if (!contains) {
      continue
    }
    const field = Object.keys(contains).find((key) => !knownFields.includes(key))
    if (field) {
      return { field, value: contains[field] }
    }
  }
  return {}
}

export const parseRules = (rules) => {
  const entries = rules?.all || rules?.any || []
  const bpmRange = getRuleValue(entries, 'inTheRange', 'bpm')
  const customTag = getCustomTagRule(entries)

  return {
    smartMode: !!rules,
    smartLovedOnly: getRuleValue(entries, 'is', 'loved') === true,
    smartTitle: getRuleValue(entries, 'contains', 'title') || '',
    smartGenre: getRuleValue(entries, 'contains', 'genre') || '',
    smartArtist: getRuleValue(entries, 'contains', 'artist') || '',
    smartBpmPreset: '',
    smartBpmMin: Array.isArray(bpmRange)
      ? bpmRange[0]
      : getRuleValue(entries, 'gte', 'bpm') || getRuleValue(entries, 'gt', 'bpm') || '',
    smartBpmMax: Array.isArray(bpmRange)
      ? bpmRange[1]
      : getRuleValue(entries, 'lte', 'bpm') || getRuleValue(entries, 'lt', 'bpm') || '',
    smartTagField: customTag.field || '',
    smartTagValue: customTag.value || '',
    smartSort: rules?.sort || 'random',
    smartOrder: rules?.order || 'asc',
    smartLimit: rules?.limit || '',
  }
}

const toNumber = (value) => {
  if (!isPresent(value)) {
    return undefined
  }
  const numberValue = Number(value)
  return Number.isNaN(numberValue) ? undefined : numberValue
}

export const bpmPresetRanges = {
  downtempo: [60, 89],
  midtempo: [90, 114],
  house: [115, 129],
  trance: [130, 144],
  fast: [145, null],
}

const addBpmRule = (all, values) => {
  const preset = bpmPresetRanges[values.smartBpmPreset]
  const bpmMin = preset ? preset[0] : toNumber(values.smartBpmMin)
  const bpmMax = preset ? preset[1] : toNumber(values.smartBpmMax)

  if (
    bpmMin !== undefined &&
    bpmMin !== null &&
    bpmMax !== undefined &&
    bpmMax !== null
  ) {
    all.push({ inTheRange: { bpm: [bpmMin, bpmMax] } })
  } else if (bpmMin !== undefined && bpmMin !== null) {
    if (values.smartBpmPreset === 'fast') {
      all.push({ gte: { bpm: bpmMin } })
    } else {
      all.push({ gt: { bpm: bpmMin } })
    }
  } else if (bpmMax !== undefined && bpmMax !== null) {
    all.push({ lt: { bpm: bpmMax } })
  }
}

export const buildSmartPlaylistRules = (values) => {
  if (!values.smartMode) {
    return null
  }

  const all = []
  if (values.smartLovedOnly) {
    all.push({ is: { loved: true } })
  }
  splitTerms(values.smartTitle).forEach((term) => {
    all.push({ contains: { title: term } })
  })

  const genres = splitTerms(values.smartGenre)
  if (genres.length === 1) {
    all.push({ contains: { genre: genres[0] } })
  } else if (genres.length > 1) {
    all.push({
      any: genres.map((genre) => ({ contains: { genre } })),
    })
  }

  if (isPresent(values.smartArtist)) {
    all.push({ contains: { artist: values.smartArtist.trim() } })
  }

  addBpmRule(all, values)

  if (isPresent(values.smartTagField) && isPresent(values.smartTagValue)) {
    all.push({
      contains: {
        [values.smartTagField.trim().toLowerCase()]: values.smartTagValue.trim(),
      },
    })
  }

  if (all.length === 0) {
    all.push({ gt: { playcount: -1 } })
  }

  const rules = {
    all,
    sort: values.smartSort || 'random',
    order: values.smartOrder || 'asc',
  }

  const limit = toNumber(values.smartLimit)
  if (limit && limit > 0) {
    rules.limit = limit
  }

  return rules
}

export const cleanPlaylistPayload = (values) => {
  const normalizedValues = { ...values }
  const defaults = parseRules(values.rules)
  helperFields.forEach((field) => {
    if (normalizedValues[field] === undefined) {
      normalizedValues[field] = defaults[field]
    }
  })
  if (
    normalizedValues.smartMode &&
    normalizedValues.smartLovedOnly === undefined
  ) {
    normalizedValues.smartLovedOnly = true
  }

  const payload = {
    ...values,
    rules: buildSmartPlaylistRules(normalizedValues),
  }
  helperFields.forEach((field) => delete payload[field])
  if (payload.rules) {
    payload.evaluatedAt = null
  } else {
    delete payload.evaluatedAt
  }
  return payload
}
