import { DateTimePickerComponent } from '@syncfusion/ej2-react-calendars';
import React from 'react';
import './TimePicker.scss';
import { t } from 'i18next';

class TimePicker extends React.PureComponent {
  render() {
    return <DateTimePickerComponent placeholder ={t('timepicker.placeholder')} id="datetimepicker" />;
  }
}
export default TimePicker;
