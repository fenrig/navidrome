import React from 'react'
import {
  BooleanInput,
  FormDataConsumer,
  NumberInput,
  SelectInput,
  TextInput,
  useTranslate,
} from 'react-admin'
import { Box, Typography } from '@material-ui/core'
import { parseRules } from './smartPlaylistRulesUtils'

const sortChoices = [
  { id: 'random', name: 'resources.playlist.smart.sort.random' },
  { id: 'title', name: 'resources.playlist.smart.sort.title' },
  { id: 'dateadded', name: 'resources.playlist.smart.sort.dateAdded' },
  { id: 'lastplayed', name: 'resources.playlist.smart.sort.lastPlayed' },
  { id: 'playcount', name: 'resources.playlist.smart.sort.playCount' },
  { id: 'rating', name: 'resources.playlist.smart.sort.rating' },
  { id: 'bpm', name: 'resources.playlist.smart.sort.bpm' },
]

const orderChoices = [
  { id: 'asc', name: 'resources.playlist.smart.order.asc' },
  { id: 'desc', name: 'resources.playlist.smart.order.desc' },
]

const bpmPresetChoices = [
  { id: '', name: 'resources.playlist.smart.bpmPreset.custom' },
  { id: 'downtempo', name: 'resources.playlist.smart.bpmPreset.downtempo' },
  { id: 'midtempo', name: 'resources.playlist.smart.bpmPreset.midtempo' },
  { id: 'house', name: 'resources.playlist.smart.bpmPreset.house' },
  { id: 'trance', name: 'resources.playlist.smart.bpmPreset.trance' },
  { id: 'fast', name: 'resources.playlist.smart.bpmPreset.fast' },
]


const SmartPlaylistRulesInner = ({ formData, defaults }) => {
  const translate = useTranslate()

  if (!(formData.smartMode ?? defaults.smartMode)) {
    return null
  }

  return (
    <Box mt={1} mb={1}>
      <Typography variant="subtitle2" gutterBottom>
        {translate('resources.playlist.smart.rules')}
      </Typography>
      <BooleanInput
        source="smartLovedOnly"
        label="resources.playlist.smart.lovedOnly"
        defaultValue={defaults.smartLovedOnly}
      />
      <TextInput
        source="smartTitle"
        label="resources.playlist.smart.title"
        helperText="resources.playlist.smart.titleHelp"
        defaultValue={defaults.smartTitle}
      />
      <TextInput
        source="smartGenre"
        label="resources.playlist.smart.genre"
        helperText="resources.playlist.smart.genreHelp"
        defaultValue={defaults.smartGenre}
      />
      <TextInput
        source="smartArtist"
        label="resources.playlist.smart.artist"
        helperText="resources.playlist.smart.artistHelp"
        defaultValue={defaults.smartArtist}
      />
      <Box display="flex" flexWrap="wrap" gridGap={16}>
        <SelectInput
          source="smartBpmPreset"
          label="resources.playlist.smart.bpmPreset.label"
          choices={bpmPresetChoices}
          defaultValue={defaults.smartBpmPreset}
        />
        <NumberInput
          source="smartBpmMin"
          label="resources.playlist.smart.bpmMin"
          defaultValue={defaults.smartBpmMin}
        />
        <NumberInput
          source="smartBpmMax"
          label="resources.playlist.smart.bpmMax"
          defaultValue={defaults.smartBpmMax}
        />
      </Box>
      <Box display="flex" flexWrap="wrap" gridGap={16}>
        <TextInput
          source="smartTagField"
          label="resources.playlist.smart.tagField"
          helperText="resources.playlist.smart.tagFieldHelp"
          defaultValue={defaults.smartTagField}
        />
        <TextInput
          source="smartTagValue"
          label="resources.playlist.smart.tagValue"
          defaultValue={defaults.smartTagValue}
        />
      </Box>
      <Box display="flex" flexWrap="wrap" gridGap={16}>
        <SelectInput
          source="smartSort"
          label="resources.playlist.smart.sortBy"
          choices={sortChoices}
          defaultValue={defaults.smartSort}
        />
        <SelectInput
          source="smartOrder"
          label="resources.playlist.smart.orderBy"
          choices={orderChoices}
          defaultValue={defaults.smartOrder}
        />
        <NumberInput
          source="smartLimit"
          label="resources.playlist.smart.limit"
          helperText="resources.playlist.smart.limitHelp"
          defaultValue={defaults.smartLimit}
        />
      </Box>
    </Box>
  )
}

const SmartPlaylistRules = ({ record }) => {
  const defaults = parseRules(record?.rules)
  if (record === undefined) {
    defaults.smartLovedOnly = true
  }

  return (
    <FormDataConsumer>
      {({ formData }) => (
        <>
          <BooleanInput
            source="smartMode"
            label="resources.playlist.smart.enabled"
            defaultValue={defaults.smartMode}
          />
          <SmartPlaylistRulesInner formData={formData} defaults={defaults} />
        </>
      )}
    </FormDataConsumer>
  )
}

export default SmartPlaylistRules
