import React, { useCallback } from 'react'
import {
  Create,
  SimpleForm,
  TextInput,
  BooleanInput,
  required,
  useTranslate,
  useRefresh,
  useNotify,
  useRedirect,
  useMutation,
} from 'react-admin'
import { Title } from '../common'
import SmartPlaylistRules from './SmartPlaylistRules'
import { cleanPlaylistPayload } from './smartPlaylistRulesUtils'

const PlaylistCreate = (props) => {
  const { basePath } = props
  const refresh = useRefresh()
  const notify = useNotify()
  const redirect = useRedirect()
  const [mutate] = useMutation()
  const translate = useTranslate()
  const resourceName = translate('resources.playlist.name', { smart_count: 1 })
  const title = translate('ra.page.create', {
    name: `${resourceName}`,
  })

  const save = useCallback(
    async (values) => {
      try {
        await mutate(
          {
            type: 'create',
            resource: 'playlist',
            payload: { data: cleanPlaylistPayload(values) },
          },
          { returnPromise: true },
        )
        notify('ra.notification.created', 'info', { smart_count: 1 })
        redirect('list', basePath)
        refresh()
      } catch (error) {
        if (error.body?.errors) {
          return error.body.errors
        }
      }
    },
    [basePath, mutate, notify, redirect, refresh],
  )

  return (
    <Create title={<Title subTitle={title} />} {...props}>
      <SimpleForm save={save} redirect="list" variant={'outlined'}>
        <TextInput source="name" validate={required()} />
        <TextInput multiline source="comment" />
        <BooleanInput source="public" initialValue={true} />
        <SmartPlaylistRules />
      </SimpleForm>
    </Create>
  )
}

export default PlaylistCreate
