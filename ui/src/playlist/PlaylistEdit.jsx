import { useCallback } from 'react'
import {
  Edit,
  FormDataConsumer,
  SimpleForm,
  TextInput,
  TextField,
  BooleanInput,
  required,
  useTranslate,
  usePermissions,
  ReferenceInput,
  SelectInput,
  useMutation,
  useNotify,
  useRedirect,
  useRefresh,
} from 'react-admin'
import { isWritable, Title } from '../common'
import SmartPlaylistRules from './SmartPlaylistRules'
import { cleanPlaylistPayload } from './smartPlaylistRulesUtils'

const SyncFragment = ({ formData, variant, ...rest }) => {
  return (
    <>
      {formData.path && <BooleanInput source="sync" {...rest} />}
      {formData.path && <TextField source="path" {...rest} />}
    </>
  )
}

const PlaylistTitle = ({ record }) => {
  const translate = useTranslate()
  const resourceName = translate('resources.playlist.name', { smart_count: 1 })
  return <Title subTitle={`${resourceName} "${record ? record.name : ''}"`} />
}

const PlaylistEditForm = (props) => {
  const { record } = props
  const { permissions } = usePermissions()
  const [mutate] = useMutation()
  const notify = useNotify()
  const redirect = useRedirect()
  const refresh = useRefresh()

  const save = useCallback(
    async (values) => {
      try {
        await mutate(
          {
            type: 'update',
            resource: 'playlist',
            payload: {
              id: record.id,
              data: cleanPlaylistPayload(values),
              previousData: record,
            },
          },
          { returnPromise: true },
        )
        notify('ra.notification.updated', 'info', { smart_count: 1 })
        redirect('list', props.basePath)
        refresh()
      } catch (error) {
        if (error.body?.errors) {
          return error.body.errors
        }
      }
    },
    [mutate, notify, props.basePath, record, redirect, refresh],
  )

  return (
    <SimpleForm save={save} redirect="list" variant={'outlined'} {...props}>
      <TextInput source="name" validate={required()} />
      <TextInput
        multiline
        minRows={3}
        source="comment"
        fullWidth
        inputProps={{
          style: { resize: 'vertical' },
        }}
      />
      {permissions === 'admin' ? (
        <ReferenceInput
          source="ownerId"
          reference="user"
          perPage={0}
          sort={{ field: 'name', order: 'ASC' }}
        >
          <SelectInput
            label={'resources.playlist.fields.ownerName'}
            optionText="userName"
          />
        </ReferenceInput>
      ) : (
        <TextField source="ownerName" />
      )}
      <BooleanInput source="public" disabled={!isWritable(record.ownerId)} />
      <FormDataConsumer>
        {(formDataProps) => <SyncFragment {...formDataProps} />}
      </FormDataConsumer>
      <SmartPlaylistRules record={record} />
    </SimpleForm>
  )
}

const PlaylistEdit = (props) => (
  <Edit title={<PlaylistTitle />} actions={false} {...props}>
    <PlaylistEditForm {...props} />
  </Edit>
)

export default PlaylistEdit
